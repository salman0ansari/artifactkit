// Package ooxml parses DOCX, PPTX, and XLSX packages without executing active content.
package ooxml

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/internal/ziputil"
	"github.com/salman0ansari/artifactkit/parser"
)

type Parser struct{}

func New() *Parser           { return &Parser{} }
func (*Parser) Name() string { return "ooxml" }

func (*Parser) Formats() []artifact.Format {
	return []artifact.Format{
		{Name: "docx", MediaType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", Extensions: []string{".docx"}},
		{Name: "pptx", MediaType: "application/vnd.openxmlformats-officedocument.presentationml.presentation", Extensions: []string{".pptx"}},
		{Name: "xlsx", MediaType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Extensions: []string{".xlsx"}},
	}
}

func (*Parser) Probe(source parser.Source) parser.Match {
	if len(source.Data) < 4 || !bytes.Equal(source.Data[:2], []byte{'P', 'K'}) {
		return parser.Match{}
	}
	archive, err := zip.NewReader(bytes.NewReader(source.Data), int64(len(source.Data)))
	if err == nil {
		var word, presentation, workbook bool
		for _, file := range archive.File {
			switch file.Name {
			case "word/document.xml":
				word = true
			case "ppt/presentation.xml":
				presentation = true
			case "xl/workbook.xml":
				workbook = true
			}
		}
		switch {
		case word:
			return parser.Match{Score: 100, Reason: "WordprocessingML package"}
		case presentation:
			return parser.Match{Score: 100, Reason: "PresentationML package"}
		case workbook:
			return parser.Match{Score: 100, Reason: "SpreadsheetML package"}
		}
	}
	switch strings.ToLower(filepath.Ext(source.Name)) {
	case ".docx", ".pptx", ".xlsx":
		return parser.Match{Score: 98, Reason: "OOXML extension"}
	}
	return parser.Match{}
}

func (*Parser) Parse(ctx context.Context, source parser.Source, limits artifact.Limits) (*artifact.Artifact, error) {
	archive, err := ziputil.Open(source.Data, limits)
	if err != nil {
		return nil, fmt.Errorf("open OOXML package: %w", err)
	}
	packageFile := &packageReader{archive: archive, source: source, limits: limits}
	switch {
	case archive.Has("word/document.xml"):
		return packageFile.parseDOCX(ctx)
	case archive.Has("ppt/presentation.xml"):
		return packageFile.parsePPTX(ctx)
	case archive.Has("xl/workbook.xml"):
		return packageFile.parseXLSX(ctx)
	default:
		return nil, fmt.Errorf("OOXML package has no supported main document part")
	}
}

type packageReader struct {
	archive *ziputil.Reader
	source  parser.Source
	limits  artifact.Limits
}
