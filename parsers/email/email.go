// Package email parses RFC 5322 messages, MIME bodies, and attachment metadata.
package email

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"path"
	"strings"

	"golang.org/x/net/html"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
)

type Parser struct{}

func New() *Parser           { return &Parser{} }
func (*Parser) Name() string { return "email" }

func (*Parser) Formats() []artifact.Format {
	return []artifact.Format{{Name: "eml", MediaType: "message/rfc822", Extensions: []string{".eml"}}}
}

func (*Parser) Probe(source parser.Source) parser.Match {
	if source.Extension() == ".eml" {
		return parser.Match{Score: 95, Reason: "EML extension"}
	}
	prefix := source.Data
	if len(prefix) > 4096 {
		prefix = prefix[:4096]
	}
	lower := bytes.ToLower(prefix)
	if bytes.Contains(lower, []byte("from:")) && bytes.Contains(lower, []byte("subject:")) && bytes.Contains(prefix, []byte("\n\n")) {
		return parser.Match{Score: 70, Reason: "RFC 5322 headers"}
	}
	return parser.Match{}
}

func (*Parser) Parse(ctx context.Context, source parser.Source, limits artifact.Limits) (*artifact.Artifact, error) {
	message, err := mail.ReadMessage(bytes.NewReader(source.Data))
	if err != nil {
		return nil, fmt.Errorf("parse email headers: %w", err)
	}
	result := &artifact.Artifact{Name: source.Name, Format: "eml", MediaType: "message/rfc822", Metadata: make(map[string]string)}
	for _, key := range []string{"From", "To", "Cc", "Date", "Message-Id"} {
		if value := message.Header.Get(key); value != "" {
			result.Metadata[strings.ToLower(key)] = value
		}
	}
	if subject := decodeHeader(message.Header.Get("Subject")); subject != "" {
		result.Metadata["subject"] = subject
	}
	root := artifact.Node{Kind: artifact.KindDocument, Name: source.Name}
	builder := messageBuilder{ctx: ctx, source: source.Data, limits: limits, result: result}
	if err := builder.part(&root, message.Header, message.Body, 1); err != nil {
		return nil, err
	}
	result.Root = root
	return result, nil
}

type messageBuilder struct {
	ctx      context.Context
	source   []byte
	limits   artifact.Limits
	result   *artifact.Artifact
	nodes    int
	expanded int64
}

func (b *messageBuilder) part(parent *artifact.Node, header mail.Header, body io.Reader, depth int) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	if depth > b.limits.MaxNestingDepth {
		return &artifact.LimitError{Limit: "nesting depth", Value: int64(depth), Max: int64(b.limits.MaxNestingDepth)}
	}
	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil || mediaType == "" {
		mediaType = "text/plain"
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return fmt.Errorf("parse email: multipart body has no boundary")
		}
		reader := multipart.NewReader(body, boundary)
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("parse email multipart: %w", err)
			}
			if err := b.part(parent, mail.Header(part.Header), part, depth+1); err != nil {
				part.Close()
				return err
			}
			part.Close()
		}
		return nil
	}

	decoded := transferReader(header.Get("Content-Transfer-Encoding"), body)
	remaining := b.limits.MaxExpandedBytes - b.expanded
	if remaining <= 0 {
		return &artifact.LimitError{Limit: "expanded bytes", Value: b.expanded + 1, Max: b.limits.MaxExpandedBytes}
	}
	data, err := io.ReadAll(io.LimitReader(decoded, remaining+1))
	if err != nil {
		return fmt.Errorf("parse email body: %w", err)
	}
	b.expanded += int64(len(data))
	if b.expanded > b.limits.MaxExpandedBytes {
		return &artifact.LimitError{Limit: "expanded bytes", Value: b.expanded, Max: b.limits.MaxExpandedBytes}
	}

	disposition, dispositionParams, _ := mime.ParseMediaType(header.Get("Content-Disposition"))
	filename := decodeHeader(dispositionParams["filename"])
	if filename == "" {
		filename = decodeHeader(params["name"])
	}
	if disposition == "attachment" || filename != "" {
		if filename == "" {
			filename = "attachment"
		}
		filename = safeFilename(filename)
		logicalPath := fmt.Sprintf("attachment/%d/%s", len(b.result.Resources)+1, filename)
		locator := artifact.Locator{Path: logicalPath}
		node := artifact.Node{Kind: artifact.KindAttachment, Name: filename, Locator: locator, Attributes: map[string]string{"media_type": mediaType, "size": fmt.Sprintf("%d", len(data))}}
		if err := b.add(parent, node); err != nil {
			return err
		}
		b.result.Resources = append(b.result.Resources, artifact.Resource{URI: artifact.ResourceURI(b.source, "attachment", logicalPath), Name: filename, MediaType: mediaType, Size: int64(len(data)), Locator: locator})
		return nil
	}

	var value string
	switch mediaType {
	case "text/plain":
		value = strings.TrimSpace(string(data))
	case "text/html":
		value = htmlText(data)
	default:
		return nil
	}
	if value != "" {
		return b.add(parent, artifact.Node{Kind: artifact.KindParagraph, Text: value, Locator: artifact.Locator{Path: "body"}, Attributes: map[string]string{"media_type": mediaType}})
	}
	return nil
}

func (b *messageBuilder) add(parent *artifact.Node, node artifact.Node) error {
	b.nodes++
	if b.nodes > b.limits.MaxNodes {
		return &artifact.LimitError{Limit: "nodes", Value: int64(b.nodes), Max: int64(b.limits.MaxNodes)}
	}
	parent.Children = append(parent.Children, node)
	return nil
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

func htmlText(data []byte) string {
	root, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return strings.TrimSpace(string(data))
	}
	var parts []string
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.ElementNode && (current.Data == "script" || current.Data == "style") {
			return
		}
		if current.Type == html.TextNode {
			parts = append(parts, current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

func decodeHeader(value string) string {
	if decoded, err := new(mime.WordDecoder).DecodeHeader(value); err == nil {
		return decoded
	}
	return value
}

func safeFilename(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = path.Base(value)
	if value == "." || value == "/" || value == "" {
		return "attachment"
	}
	return strings.ReplaceAll(value, "\x00", "")
}
