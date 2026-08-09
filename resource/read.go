package resource

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strconv"
	"strings"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/internal/ziputil"
)

func readStored(ctx context.Context, format string, source []byte, item artifact.Resource, offset, limit int64, limits artifact.Limits) (*Content, error) {
	switch format {
	case "zip", "docx", "pptx", "xlsx":
		archive, err := ziputil.Open(source, limits)
		if err != nil {
			return nil, err
		}
		file, exists := archive.File(item.Locator.Path)
		if !exists {
			return nil, ErrNotFound
		}
		reader, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return readRange(ctx, reader, item.Size, offset, limit)
	case "tar":
		return readTAR(ctx, tar.NewReader(bytes.NewReader(source)), item, offset, limit)
	case "tar.gz":
		reader, err := gzip.NewReader(bytes.NewReader(source))
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return readTAR(ctx, tar.NewReader(&contextReader{ctx: ctx, reader: reader}), item, offset, limit)
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(source))
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return readRange(ctx, reader, item.Size, offset, limit)
	case "eml":
		return readEmailAttachment(ctx, source, item, offset, limit, limits)
	default:
		return nil, fmt.Errorf("artifactkit: lazy reads are not implemented for %s", format)
	}
}

func readTAR(ctx context.Context, reader *tar.Reader, item artifact.Resource, offset, limit int64) (*Content, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		header, err := reader.Next()
		if err == io.EOF {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		name, err := ziputil.CleanPath(header.Name)
		if err != nil {
			return nil, err
		}
		if name == item.Locator.Path {
			return readRange(ctx, reader, header.Size, offset, limit)
		}
	}
}

func readRange(ctx context.Context, reader io.Reader, size, offset, limit int64) (*Content, error) {
	if offset >= size {
		return &Content{Data: []byte{}, EOF: true}, nil
	}
	reader = &contextReader{ctx: ctx, reader: reader}
	if offset > 0 {
		if _, err := io.CopyN(io.Discard, reader, offset); err != nil {
			return nil, fmt.Errorf("artifactkit: seek resource offset: %w", err)
		}
	}
	remaining := size - offset
	if limit > remaining {
		limit = remaining
	}
	data, err := io.ReadAll(io.LimitReader(reader, limit))
	if err != nil {
		return nil, err
	}
	if limit == remaining && int64(len(data)) < remaining {
		return nil, fmt.Errorf("artifactkit: resource ended before its declared size")
	}
	return &Content{Data: data, EOF: int64(len(data)) >= remaining}, nil
}

func readEmailAttachment(ctx context.Context, source []byte, item artifact.Resource, offset, limit int64, limits artifact.Limits) (*Content, error) {
	message, err := mail.ReadMessage(bytes.NewReader(source))
	if err != nil {
		return nil, err
	}
	target := attachmentIndex(item.Locator.Path)
	if target <= 0 {
		return nil, ErrNotFound
	}
	index := 0
	var walk func(mail.Header, io.Reader, int) (*Content, error)
	walk = func(header mail.Header, body io.Reader, depth int) (*Content, error) {
		if depth > limits.MaxNestingDepth {
			return nil, &artifact.LimitError{Limit: "nesting depth", Value: int64(depth), Max: int64(limits.MaxNestingDepth)}
		}
		mediaType, params, parseErr := mime.ParseMediaType(header.Get("Content-Type"))
		if parseErr != nil || mediaType == "" {
			mediaType = "text/plain"
		}
		if strings.HasPrefix(mediaType, "multipart/") {
			reader := multipart.NewReader(body, params["boundary"])
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					return nil, ErrNotFound
				}
				if err != nil {
					return nil, err
				}
				content, err := walk(mail.Header(part.Header), part, depth+1)
				part.Close()
				if err == nil {
					return content, nil
				}
				if !errors.Is(err, ErrNotFound) {
					return nil, err
				}
			}
		}
		disposition, dispositionParams, _ := mime.ParseMediaType(header.Get("Content-Disposition"))
		filename := dispositionParams["filename"]
		if filename == "" {
			filename = params["name"]
		}
		if disposition != "attachment" && filename == "" {
			return nil, ErrNotFound
		}
		index++
		if index != target {
			return nil, ErrNotFound
		}
		return readRange(ctx, transferReader(header.Get("Content-Transfer-Encoding"), body), item.Size, offset, limit)
	}
	return walk(message.Header, message.Body, 1)
}

func attachmentIndex(locator string) int {
	parts := strings.Split(locator, "/")
	if len(parts) < 3 || parts[0] != "attachment" {
		return 0
	}
	value, _ := strconv.Atoi(parts[1])
	return value
}

func transferReader(encoding string, reader io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, reader)
	case "quoted-printable":
		return quotedprintable.NewReader(reader)
	default:
		return reader
	}
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
