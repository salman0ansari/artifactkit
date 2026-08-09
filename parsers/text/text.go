// Package text parses UTF-8 plain text and Markdown documents.
package text

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
)

type Parser struct{}

func New() *Parser           { return &Parser{} }
func (*Parser) Name() string { return "text" }

func (*Parser) Formats() []artifact.Format {
	return []artifact.Format{
		{Name: "markdown", MediaType: "text/markdown", Extensions: []string{".md", ".markdown"}},
		{Name: "text", MediaType: "text/plain", Extensions: []string{".txt", ".text", ".log"}},
	}
}

func (*Parser) Probe(source parser.Source) parser.Match {
	if bytes.IndexByte(source.Data, 0) >= 0 || !utf8.Valid(source.Data) {
		return parser.Match{}
	}
	switch source.Extension() {
	case ".md", ".markdown":
		return parser.Match{Score: 85, Reason: "markdown extension and valid UTF-8"}
	case ".txt", ".text", ".log":
		return parser.Match{Score: 75, Reason: "text extension and valid UTF-8"}
	}
	if strings.HasPrefix(source.MediaType, "text/") {
		return parser.Match{Score: 45, Reason: "text media type"}
	}
	if len(source.Data) > 0 {
		return parser.Match{Score: 20, Reason: "valid UTF-8 fallback"}
	}
	return parser.Match{Score: 10, Reason: "empty text input"}
}

func (*Parser) Parse(ctx context.Context, source parser.Source, limits artifact.Limits) (*artifact.Artifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	format := "text"
	mediaType := "text/plain"
	if ext := strings.ToLower(filepath.Ext(source.Name)); ext == ".md" || ext == ".markdown" {
		format = "markdown"
		mediaType = "text/markdown"
	}

	root := artifact.Node{Kind: artifact.KindDocument, Name: source.Name}
	if format == "markdown" {
		root.Children = markdownNodes(source.Data)
	} else {
		root.Children = paragraphNodes(source.Data)
	}
	result := &artifact.Artifact{
		Name: source.Name, Format: format, MediaType: mediaType, Root: root,
		Metadata: map[string]string{"encoding": "utf-8"},
	}
	if format == "markdown" {
		for _, n := range root.Children {
			if n.Kind == artifact.KindSection && n.Name != "" {
				result.Metadata["title"] = n.Name
				break
			}
		}
	}
	return result, nil
}

type line struct {
	text       string
	start, end int64
}

func splitLines(data []byte) []line {
	lines := make([]line, 0, bytes.Count(data, []byte{'\n'})+1)
	for start := 0; start < len(data); {
		rel := bytes.IndexByte(data[start:], '\n')
		end := len(data)
		next := len(data)
		if rel >= 0 {
			end = start + rel
			next = end + 1
		}
		textEnd := end
		if textEnd > start && data[textEnd-1] == '\r' {
			textEnd--
		}
		lines = append(lines, line{text: string(data[start:textEnd]), start: int64(start), end: int64(end)})
		start = next
	}
	if len(data) == 0 {
		return nil
	}
	return lines
}

func paragraphNodes(data []byte) []artifact.Node {
	lines := splitLines(data)
	var nodes []artifact.Node
	for i := 0; i < len(lines); {
		for i < len(lines) && strings.TrimSpace(lines[i].text) == "" {
			i++
		}
		if i >= len(lines) {
			break
		}
		start := i
		for i < len(lines) && strings.TrimSpace(lines[i].text) != "" {
			i++
		}
		parts := make([]string, 0, i-start)
		for _, current := range lines[start:i] {
			parts = append(parts, current.text)
		}
		nodes = append(nodes, artifact.Node{
			Kind:    artifact.KindParagraph,
			Text:    strings.Join(parts, "\n"),
			Locator: artifact.Locator{ByteRange: &artifact.ByteRange{Start: lines[start].start, End: lines[i-1].end}},
		})
	}
	return nodes
}

func markdownNodes(data []byte) []artifact.Node {
	lines := splitLines(data)
	var roots []artifact.Node
	var section *artifact.Node
	var paragraph []line

	flush := func() {
		if len(paragraph) == 0 {
			return
		}
		parts := make([]string, 0, len(paragraph))
		for _, current := range paragraph {
			parts = append(parts, current.text)
		}
		node := artifact.Node{
			Kind:    artifact.KindParagraph,
			Text:    strings.Join(parts, "\n"),
			Locator: artifact.Locator{ByteRange: &artifact.ByteRange{Start: paragraph[0].start, End: paragraph[len(paragraph)-1].end}},
		}
		if section != nil {
			section.Children = append(section.Children, node)
		} else {
			roots = append(roots, node)
		}
		paragraph = nil
	}

	for _, current := range lines {
		level, title := heading(current.text)
		if level > 0 {
			flush()
			roots = append(roots, artifact.Node{
				Kind: artifact.KindSection, Name: title, Level: level,
				Locator: artifact.Locator{ByteRange: &artifact.ByteRange{Start: current.start, End: current.end}},
			})
			section = &roots[len(roots)-1]
			continue
		}
		if strings.TrimSpace(current.text) == "" {
			flush()
			continue
		}
		paragraph = append(paragraph, current)
	}
	flush()
	return roots
}

func heading(value string) (int, string) {
	trimmed := strings.TrimLeft(value, " \t")
	level := 0
	for level < len(trimmed) && level < 6 && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level >= len(trimmed) || trimmed[level] != ' ' {
		return 0, ""
	}
	return level, strings.TrimSpace(strings.TrimRight(trimmed[level+1:], "#"))
}
