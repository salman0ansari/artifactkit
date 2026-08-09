// Package archive safely inventories ZIP, TAR, and GZIP containers.
package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/internal/ziputil"
	"github.com/salman0ansari/artifactkit/parser"
)

type Parser struct{}

func New() *Parser           { return &Parser{} }
func (*Parser) Name() string { return "archive" }

func (*Parser) Formats() []artifact.Format {
	return []artifact.Format{
		{Name: "gzip", MediaType: "application/gzip", Extensions: []string{".gz"}},
		{Name: "tar", MediaType: "application/x-tar", Extensions: []string{".tar"}},
		{Name: "tar.gz", MediaType: "application/gzip", Extensions: []string{".tar.gz", ".tgz"}},
		{Name: "zip", MediaType: "application/zip", Extensions: []string{".zip"}},
	}
}

func (*Parser) Probe(source parser.Source) parser.Match {
	switch {
	case isZIP(source.Data):
		return parser.Match{Score: 95, Reason: "ZIP signature"}
	case isGZIP(source.Data):
		return parser.Match{Score: 95, Reason: "GZIP signature"}
	case isTAR(source.Data):
		return parser.Match{Score: 95, Reason: "TAR ustar signature"}
	}
	return parser.Match{}
}

func (*Parser) Parse(ctx context.Context, source parser.Source, limits artifact.Limits) (*artifact.Artifact, error) {
	switch {
	case isZIP(source.Data):
		return parseZIP(ctx, source, limits)
	case isGZIP(source.Data):
		if isTarGZIP(source) {
			return parseTarGZIP(ctx, source, limits)
		}
		return parseGZIP(ctx, source, limits)
	case isTAR(source.Data):
		return parseTAR(ctx, source, limits)
	default:
		return nil, fmt.Errorf("unknown archive encoding")
	}
}

func parseZIP(ctx context.Context, source parser.Source, limits artifact.Limits) (*artifact.Artifact, error) {
	reader, err := ziputil.Open(source.Data, limits)
	if err != nil {
		return nil, fmt.Errorf("parse ZIP: %w", err)
	}
	result, archiveNode := newArchive(source, "zip", "application/zip")
	for _, archiveEntry := range reader.Entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name, file := archiveEntry.Name, archiveEntry.File
		size := int64(file.UncompressedSize64)
		locator := artifact.Locator{Path: name}
		attributes := map[string]string{
			"size":            strconv.FormatInt(size, 10),
			"compressed_size": strconv.FormatUint(file.CompressedSize64, 10),
			"method":          strconv.Itoa(int(file.Method)),
		}
		entry := artifact.Node{Kind: artifact.KindEntry, Name: name, Locator: locator, Attributes: attributes}
		if file.FileInfo().IsDir() {
			attributes["type"] = "directory"
		} else if file.Mode()&os.ModeSymlink != 0 {
			attributes["type"] = "symlink"
			result.Warnings = append(result.Warnings, artifact.Warning{Code: "archive_symlink_skipped", Message: "symlink entry was not exposed as a readable resource", Locator: locator})
		} else if file.Flags&1 != 0 {
			attributes["encrypted"] = "true"
			result.Warnings = append(result.Warnings, artifact.Warning{Code: "encrypted_entry_skipped", Message: "encrypted ZIP entry requires credentials and was not exposed", Locator: locator})
		} else {
			result.Resources = append(result.Resources, artifact.Resource{URI: artifact.ResourceURI(source.Data, "entry", name), Name: name, MediaType: contentType(name), Size: size, Locator: locator})
		}
		archiveNode.Children = append(archiveNode.Children, entry)
		if len(archiveNode.Children)+2 > limits.MaxNodes {
			return nil, &artifact.LimitError{Limit: "nodes", Value: int64(len(archiveNode.Children) + 2), Max: int64(limits.MaxNodes)}
		}
	}
	archiveNode.Attributes = map[string]string{"entries": strconv.Itoa(len(reader.Entries)), "expanded_size": strconv.FormatInt(reader.Expanded, 10)}
	return result, nil
}

func parseTAR(ctx context.Context, source parser.Source, limits artifact.Limits) (*artifact.Artifact, error) {
	result, archiveNode := newArchive(source, "tar", "application/x-tar")
	if err := readTAR(ctx, tar.NewReader(bytes.NewReader(source.Data)), source, limits, result, archiveNode); err != nil {
		return nil, err
	}
	return result, nil
}

func parseTarGZIP(ctx context.Context, source parser.Source, limits artifact.Limits) (*artifact.Artifact, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(source.Data))
	if err != nil {
		return nil, fmt.Errorf("parse GZIP: %w", err)
	}
	defer gzipReader.Close()
	counter := &countingReader{reader: &contextReader{ctx: ctx, reader: gzipReader}}
	bounded := io.LimitReader(counter, limits.MaxExpandedBytes+1)
	result, archiveNode := newArchive(source, "tar.gz", "application/gzip")
	if err := readTAR(ctx, tar.NewReader(bounded), source, limits, result, archiveNode); err != nil {
		return nil, err
	}
	if counter.count > limits.MaxExpandedBytes {
		return nil, &artifact.LimitError{Limit: "expanded bytes", Value: counter.count, Max: limits.MaxExpandedBytes}
	}
	if err := ziputil.CheckRatio(counter.count, int64(len(source.Data)), limits); err != nil {
		return nil, fmt.Errorf("parse tar.gz: %w", err)
	}
	return result, nil
}

func readTAR(ctx context.Context, reader *tar.Reader, source parser.Source, limits artifact.Limits, result *artifact.Artifact, archiveNode *artifact.Node) error {
	var expanded int64
	entries := 0
	seen := make(map[string]struct{})
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("parse TAR: %w", err)
		}
		entries++
		if entries > limits.MaxArchiveEntries {
			return &artifact.LimitError{Limit: "archive entries", Value: int64(entries), Max: int64(limits.MaxArchiveEntries)}
		}
		name, err := ziputil.CleanPath(header.Name)
		if err != nil {
			return fmt.Errorf("parse TAR: %w", err)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("parse TAR: duplicate archive path %q", name)
		}
		seen[name] = struct{}{}
		if header.Size < 0 || header.Size > limits.MaxExpandedBytes-expanded {
			return &artifact.LimitError{Limit: "expanded bytes", Value: expanded + max64(header.Size, 0), Max: limits.MaxExpandedBytes}
		}
		expanded += header.Size
		locator := artifact.Locator{Path: name}
		attributes := map[string]string{"size": strconv.FormatInt(header.Size, 10), "mode": fmt.Sprintf("%#o", header.Mode)}
		entry := artifact.Node{Kind: artifact.KindEntry, Name: name, Locator: locator, Attributes: attributes}
		switch header.Typeflag {
		case tar.TypeDir:
			attributes["type"] = "directory"
		case tar.TypeReg, tar.TypeRegA:
			result.Resources = append(result.Resources, artifact.Resource{URI: artifact.ResourceURI(source.Data, "entry", name), Name: name, MediaType: contentType(name), Size: header.Size, Locator: locator})
		case tar.TypeSymlink, tar.TypeLink:
			attributes["type"] = "link"
			result.Warnings = append(result.Warnings, artifact.Warning{Code: "archive_link_skipped", Message: "archive link was not exposed as a readable resource", Locator: locator})
		default:
			attributes["type"] = "special"
			result.Warnings = append(result.Warnings, artifact.Warning{Code: "archive_special_skipped", Message: "special archive entry was not exposed as a readable resource", Locator: locator})
		}
		archiveNode.Children = append(archiveNode.Children, entry)
		if len(archiveNode.Children)+2 > limits.MaxNodes {
			return &artifact.LimitError{Limit: "nodes", Value: int64(len(archiveNode.Children) + 2), Max: int64(limits.MaxNodes)}
		}
	}
	archiveNode.Attributes = map[string]string{"entries": strconv.Itoa(entries), "expanded_size": strconv.FormatInt(expanded, 10)}
	return nil
}

func parseGZIP(ctx context.Context, source parser.Source, limits artifact.Limits) (*artifact.Artifact, error) {
	reader, err := gzip.NewReader(bytes.NewReader(source.Data))
	if err != nil {
		return nil, fmt.Errorf("parse GZIP: %w", err)
	}
	defer reader.Close()
	count, err := io.Copy(io.Discard, io.LimitReader(&contextReader{ctx: ctx, reader: reader}, limits.MaxExpandedBytes+1))
	if err != nil {
		return nil, fmt.Errorf("parse GZIP: %w", err)
	}
	if count > limits.MaxExpandedBytes {
		return nil, &artifact.LimitError{Limit: "expanded bytes", Value: count, Max: limits.MaxExpandedBytes}
	}
	if err := ziputil.CheckRatio(count, int64(len(source.Data)), limits); err != nil {
		return nil, fmt.Errorf("parse GZIP: %w", err)
	}
	name := reader.Name
	if name == "" {
		name = strings.TrimSuffix(source.Name, ".gz")
	}
	name, err = ziputil.CleanPath(name)
	if err != nil {
		return nil, fmt.Errorf("parse GZIP: %w", err)
	}
	result, archiveNode := newArchive(source, "gzip", "application/gzip")
	locator := artifact.Locator{Path: name}
	attributes := map[string]string{"size": strconv.FormatInt(count, 10)}
	if !reader.ModTime.Equal(time.Time{}) {
		attributes["modified"] = reader.ModTime.UTC().Format(time.RFC3339)
	}
	archiveNode.Children = append(archiveNode.Children, artifact.Node{Kind: artifact.KindEntry, Name: name, Locator: locator, Attributes: attributes})
	archiveNode.Attributes = map[string]string{"entries": "1", "expanded_size": strconv.FormatInt(count, 10)}
	result.Resources = append(result.Resources, artifact.Resource{URI: artifact.ResourceURI(source.Data, "entry", name), Name: name, MediaType: contentType(name), Size: count, Locator: locator})
	return result, nil
}

func newArchive(source parser.Source, format, mediaType string) (*artifact.Artifact, *artifact.Node) {
	result := &artifact.Artifact{Name: source.Name, Format: format, MediaType: mediaType}
	archiveNode := artifact.Node{Kind: artifact.KindArchive, Name: source.Name}
	result.Root = artifact.Node{Kind: artifact.KindDocument, Name: source.Name, Children: []artifact.Node{archiveNode}}
	return result, &result.Root.Children[0]
}

func contentType(name string) string {
	extension := strings.ToLower(path.Ext(name))
	return map[string]string{
		".csv": "text/csv", ".eml": "message/rfc822", ".gif": "image/gif", ".htm": "text/html", ".html": "text/html",
		".jpeg": "image/jpeg", ".jpg": "image/jpeg", ".json": "application/json", ".md": "text/markdown", ".pdf": "application/pdf",
		".png": "image/png", ".tsv": "text/tab-separated-values", ".txt": "text/plain", ".xml": "application/xml",
	}[extension]
}

func isZIP(data []byte) bool {
	return len(data) >= 4 && (bytes.Equal(data[:4], []byte{'P', 'K', 3, 4}) || bytes.Equal(data[:4], []byte{'P', 'K', 5, 6}) || bytes.Equal(data[:4], []byte{'P', 'K', 7, 8}))
}
func isGZIP(data []byte) bool { return len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b }
func isTAR(data []byte) bool  { return len(data) >= 262 && bytes.Equal(data[257:262], []byte("ustar")) }

func isTarGZIP(source parser.Source) bool {
	lower := strings.ToLower(source.Name)
	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		return true
	}
	reader, err := gzip.NewReader(bytes.NewReader(source.Data))
	if err != nil {
		return false
	}
	defer reader.Close()
	header := make([]byte, 512)
	if _, err := io.ReadFull(reader, header); err != nil {
		return false
	}
	return isTAR(header)
}

type countingReader struct {
	reader io.Reader
	count  int64
}

func (r *countingReader) Read(buffer []byte) (int, error) {
	n, err := r.reader.Read(buffer)
	r.count += int64(n)
	return n, err
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
