// Package pdfparser extracts page-scoped text from PDF files using a pure-Go reader.
package pdfparser

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	pdf "github.com/ledongthuc/pdf"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
)

type Parser struct{}

func New() *Parser           { return &Parser{} }
func (*Parser) Name() string { return "pdf" }

func (*Parser) Formats() []artifact.Format {
	return []artifact.Format{{Name: "pdf", MediaType: "application/pdf", Extensions: []string{".pdf"}}}
}

func (*Parser) Probe(source parser.Source) parser.Match {
	if len(source.Data) >= 5 && bytes.Equal(source.Data[:5], []byte("%PDF-")) {
		return parser.Match{Score: 100, Reason: "PDF header"}
	}
	if source.Extension() == ".pdf" {
		return parser.Match{Score: 80, Reason: "PDF extension"}
	}
	return parser.Match{}
}

func (*Parser) Parse(ctx context.Context, source parser.Source, limits artifact.Limits) (document *artifact.Artifact, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			document = nil
			err = fmt.Errorf("parse PDF: %v", recovered)
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reader, err := pdf.NewReader(bytes.NewReader(source.Data), int64(len(source.Data)))
	if err != nil {
		return nil, err
	}
	pages := reader.NumPage()
	if pages+1 > limits.MaxNodes {
		return nil, &artifact.LimitError{Limit: "nodes", Value: int64(pages + 1), Max: int64(limits.MaxNodes)}
	}
	document = &artifact.Artifact{
		Name: source.Name, Format: "pdf", MediaType: "application/pdf",
		Metadata: map[string]string{"pages": strconv.Itoa(pages)},
		Root:     artifact.Node{Kind: artifact.KindDocument, Name: source.Name},
	}
	var textBytes int64
	for pageNumber := 1; pageNumber <= pages; pageNumber++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		text, pageErr := reader.Page(pageNumber).GetPlainText(nil)
		text = normalizeText(text)
		pageNode := artifact.Node{Kind: artifact.KindPage, Name: fmt.Sprintf("Page %d", pageNumber), Text: text, Locator: artifact.Locator{Page: pageNumber}}
		if pageErr != nil {
			document.Warnings = append(document.Warnings, artifact.Warning{Code: "pdf_page_text_error", Message: pageErr.Error(), Locator: pageNode.Locator})
		}
		if text == "" && pageErr == nil {
			document.Warnings = append(document.Warnings, artifact.Warning{Code: "pdf_page_has_no_text", Message: "page may contain only images or vector content", Locator: pageNode.Locator})
		}
		textBytes += int64(len(text))
		if textBytes > limits.MaxTextBytes {
			return nil, &artifact.LimitError{Limit: "text bytes", Value: textBytes, Max: limits.MaxTextBytes}
		}
		document.Root.Children = append(document.Root.Children, pageNode)
	}
	return document, nil
}

func normalizeText(value string) string {
	value = strings.ReplaceAll(value, "\x00", "")
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
