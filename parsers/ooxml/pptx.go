package ooxml

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/salman0ansari/artifactkit/artifact"
)

func (p *packageReader) parsePPTX(ctx context.Context) (*artifact.Artifact, error) {
	parts, err := p.presentationSlideParts()
	if err != nil {
		return nil, err
	}
	result := &artifact.Artifact{
		Name: p.source.Name, Format: "pptx",
		MediaType: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		Metadata:  p.metadata(), Root: artifact.Node{Kind: artifact.KindDocument, Name: p.source.Name},
	}
	for index, part := range parts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		nodes, err := p.parseDrawingPart(ctx, part, index+1)
		if err != nil {
			return nil, fmt.Errorf("parse PPTX slide %d: %w", index+1, err)
		}
		slide := artifact.Node{Kind: artifact.KindSlide, Name: fmt.Sprintf("Slide %d", index+1), Locator: artifact.Locator{Path: part, Slide: index + 1}, Children: nodes}
		for _, node := range nodes {
			if node.Kind == artifact.KindParagraph && strings.TrimSpace(node.Text) != "" {
				slide.Name = node.Text
				break
			}
		}
		relationships, err := p.relationships(part)
		if err != nil {
			return nil, err
		}
		for _, rel := range relationships {
			if rel.External || !strings.HasSuffix(rel.Type, "/notesSlide") || !p.archive.Has(rel.Target) {
				continue
			}
			notes, err := p.parseDrawingPart(ctx, rel.Target, index+1)
			if err != nil {
				return nil, fmt.Errorf("parse PPTX notes for slide %d: %w", index+1, err)
			}
			if len(notes) > 0 {
				slide.Children = append(slide.Children, artifact.Node{Kind: artifact.KindSection, Name: "Speaker notes", Level: 2, Locator: artifact.Locator{Path: rel.Target, Slide: index + 1}, Children: notes})
			}
		}
		result.Root.Children = append(result.Root.Children, slide)
	}
	p.packageResources(result, "ppt/media/", "ppt/embeddings/")
	if result.Metadata == nil {
		result.Metadata = make(map[string]string)
	}
	result.Metadata["slides"] = strconv.Itoa(len(parts))
	return result, nil
}

func (p *packageReader) presentationSlideParts() ([]string, error) {
	relationships, err := p.relationships("ppt/presentation.xml")
	if err != nil {
		return nil, err
	}
	data, err := p.archive.Read("ppt/presentation.xml")
	if err != nil {
		return nil, err
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var parts []string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "sldId" {
			continue
		}
		if rel, exists := relationships[relationshipID(start)]; exists && !rel.External && p.archive.Has(rel.Target) {
			parts = append(parts, rel.Target)
		}
	}
	if len(parts) > 0 {
		return parts, nil
	}
	for _, entry := range p.archive.Entries {
		if strings.HasPrefix(entry.Name, "ppt/slides/slide") && strings.HasSuffix(entry.Name, ".xml") && !strings.Contains(entry.Name, "/_rels/") {
			parts = append(parts, entry.Name)
		}
	}
	sort.Slice(parts, func(i, j int) bool { return numberedPart(parts[i]) < numberedPart(parts[j]) })
	return parts, nil
}

func (p *packageReader) parseDrawingPart(ctx context.Context, part string, slide int) ([]artifact.Node, error) {
	data, err := p.archive.Read(part)
	if err != nil {
		return nil, err
	}
	relationships, err := p.relationships(part)
	if err != nil {
		return nil, err
	}
	builder := drawingBuilder{ctx: ctx, part: part, slide: slide, relationships: relationships, limits: p.limits}
	if err := builder.parse(data); err != nil {
		return nil, err
	}
	return builder.nodes, nil
}

type drawingParagraph struct {
	text  strings.Builder
	path  string
	links []string
}

type drawingTable struct {
	node artifact.Node
	row  *artifact.Node
	cell *artifact.Node
}

type drawingBuilder struct {
	ctx           context.Context
	part          string
	slide         int
	relationships map[string]relationship
	limits        artifact.Limits
	nodes         []artifact.Node
	paragraph     *drawingParagraph
	tables        []*drawingTable
	inText        bool
	paragraphs    int
	tableCount    int
	nodeCount     int
}

func (b *drawingBuilder) parse(data []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if err := b.start(typed); err != nil {
				return err
			}
		case xml.CharData:
			if b.inText && b.paragraph != nil {
				b.paragraph.text.Write([]byte(typed))
			}
		case xml.EndElement:
			if err := b.end(typed); err != nil {
				return err
			}
		}
	}
}

func (b *drawingBuilder) start(element xml.StartElement) error {
	switch element.Name.Local {
	case "tbl":
		b.tableCount++
		locator := artifact.Locator{Path: b.part, Subpath: fmt.Sprintf("/slide/table[%d]", b.tableCount), Slide: b.slide}
		b.tables = append(b.tables, &drawingTable{node: artifact.Node{Kind: artifact.KindTable, Name: "Table " + strconv.Itoa(b.tableCount), Locator: locator}})
		return b.addNode()
	case "tr":
		if table := b.currentTable(); table != nil {
			row := len(table.node.Children) + 1
			table.row = &artifact.Node{Kind: artifact.KindRow, Name: strconv.Itoa(row), Locator: artifact.Locator{Path: b.part, Subpath: table.node.Locator.Subpath + fmt.Sprintf("/row[%d]", row), Slide: b.slide}}
			return b.addNode()
		}
	case "tc":
		if table := b.currentTable(); table != nil && table.row != nil {
			column := len(table.row.Children) + 1
			row, _ := strconv.Atoi(table.row.Name)
			cell := spreadsheetCell(column, row)
			table.cell = &artifact.Node{Kind: artifact.KindCell, Name: cell, Locator: artifact.Locator{Path: b.part, Subpath: table.row.Locator.Subpath + fmt.Sprintf("/cell[%d]", column), Slide: b.slide, Cell: cell}}
			return b.addNode()
		}
	case "p":
		b.paragraphs++
		b.paragraph = &drawingParagraph{path: fmt.Sprintf("/slide/paragraph[%d]", b.paragraphs)}
	case "t":
		b.inText = true
	case "br":
		if b.paragraph != nil {
			b.paragraph.text.WriteByte('\n')
		}
	case "hlinkClick":
		if b.paragraph != nil {
			if rel, ok := b.relationships[relationshipID(element)]; ok {
				b.paragraph.links = append(b.paragraph.links, rel.Target)
			}
		}
	}
	return nil
}

func (b *drawingBuilder) end(element xml.EndElement) error {
	switch element.Name.Local {
	case "t":
		b.inText = false
	case "p":
		return b.finishParagraph()
	case "tc":
		if table := b.currentTable(); table != nil && table.row != nil && table.cell != nil {
			table.row.Children = append(table.row.Children, *table.cell)
			table.cell = nil
		}
	case "tr":
		if table := b.currentTable(); table != nil && table.row != nil {
			table.node.Children = append(table.node.Children, *table.row)
			table.row = nil
		}
	case "tbl":
		if len(b.tables) == 0 {
			return nil
		}
		finished := b.tables[len(b.tables)-1]
		b.tables = b.tables[:len(b.tables)-1]
		if parent := b.currentTable(); parent != nil && parent.cell != nil {
			parent.cell.Children = append(parent.cell.Children, finished.node)
		} else {
			b.nodes = append(b.nodes, finished.node)
		}
	}
	return nil
}

func (b *drawingBuilder) finishParagraph() error {
	if b.paragraph == nil {
		return nil
	}
	text := strings.TrimSpace(b.paragraph.text.String())
	paragraph := b.paragraph
	b.paragraph = nil
	if text == "" {
		return nil
	}
	if table := b.currentTable(); table != nil && table.cell != nil {
		if table.cell.Text != "" {
			table.cell.Text += "\n"
		}
		table.cell.Text += text
		return nil
	}
	node := artifact.Node{Kind: artifact.KindParagraph, Text: text, Locator: artifact.Locator{Path: b.part, Subpath: paragraph.path, Slide: b.slide}}
	if len(paragraph.links) > 0 {
		node.Attributes = map[string]string{"links": strings.Join(paragraph.links, " ")}
	}
	b.nodes = append(b.nodes, node)
	return b.addNode()
}

func (b *drawingBuilder) currentTable() *drawingTable {
	if len(b.tables) == 0 {
		return nil
	}
	return b.tables[len(b.tables)-1]
}

func (b *drawingBuilder) addNode() error {
	b.nodeCount++
	if b.nodeCount > b.limits.MaxNodes {
		return &artifact.LimitError{Limit: "nodes", Value: int64(b.nodeCount), Max: int64(b.limits.MaxNodes)}
	}
	return nil
}

func numberedPart(value string) int {
	base := path.Base(value)
	base = strings.TrimSuffix(base, path.Ext(base))
	for len(base) > 0 && (base[0] < '0' || base[0] > '9') {
		base = base[1:]
	}
	number, _ := strconv.Atoi(base)
	return number
}
