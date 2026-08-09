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

func (p *packageReader) parseDOCX(ctx context.Context) (*artifact.Artifact, error) {
	result := &artifact.Artifact{
		Name: p.source.Name, Format: "docx",
		MediaType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		Metadata:  p.metadata(), Root: artifact.Node{Kind: artifact.KindDocument, Name: p.source.Name},
	}
	mainNodes, err := p.parseWordPart(ctx, "word/document.xml")
	if err != nil {
		return nil, err
	}
	result.Root.Children = append(result.Root.Children, mainNodes...)

	var auxiliary []string
	for _, entry := range p.archive.Entries {
		name := entry.Name
		base := path.Base(name)
		if strings.HasPrefix(name, "word/") && strings.HasSuffix(name, ".xml") &&
			(strings.HasPrefix(base, "header") || strings.HasPrefix(base, "footer") || base == "footnotes.xml" || base == "endnotes.xml" || base == "comments.xml") {
			auxiliary = append(auxiliary, name)
		}
	}
	sort.Strings(auxiliary)
	for _, part := range auxiliary {
		nodes, err := p.parseWordPart(ctx, part)
		if err != nil {
			return nil, err
		}
		if len(nodes) == 0 {
			continue
		}
		section := artifact.Node{Kind: artifact.KindSection, Name: wordPartLabel(part), Level: 2, Locator: artifact.Locator{Path: part}, Children: nodes}
		result.Root.Children = append(result.Root.Children, section)
	}
	p.packageResources(result, "word/media/", "word/embeddings/")
	if result.Metadata == nil {
		result.Metadata = make(map[string]string)
	}
	result.Metadata["parts"] = strconv.Itoa(1 + len(auxiliary))
	return result, nil
}

func (p *packageReader) parseWordPart(ctx context.Context, part string) ([]artifact.Node, error) {
	data, err := p.archive.Read(part)
	if err != nil {
		return nil, fmt.Errorf("read DOCX part %s: %w", part, err)
	}
	relationships, err := p.relationships(part)
	if err != nil {
		return nil, err
	}
	builder := wordBuilder{ctx: ctx, part: part, relationships: relationships, limits: p.limits}
	if err := builder.parse(data); err != nil {
		return nil, fmt.Errorf("parse DOCX part %s: %w", part, err)
	}
	return builder.nodes, nil
}

type wordParagraph struct {
	text  strings.Builder
	style string
	path  string
	links []string
}

type wordTable struct {
	node artifact.Node
	row  *artifact.Node
	cell *artifact.Node
}

type wordBuilder struct {
	ctx           context.Context
	part          string
	relationships map[string]relationship
	limits        artifact.Limits
	nodes         []artifact.Node
	paragraph     *wordParagraph
	tables        []*wordTable
	inText        bool
	paragraphs    int
	tableCount    int
	nodeCount     int
}

func (b *wordBuilder) parse(data []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		token, err := decoder.Token()
		if err == io.EOF {
			break
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
	return nil
}

func (b *wordBuilder) start(element xml.StartElement) error {
	switch element.Name.Local {
	case "tbl":
		b.tableCount++
		locator := artifact.Locator{Path: b.part, Subpath: fmt.Sprintf("/document/table[%d]", b.tableCount)}
		b.tables = append(b.tables, &wordTable{node: artifact.Node{Kind: artifact.KindTable, Name: "Table " + strconv.Itoa(b.tableCount), Locator: locator}})
		return b.addNode()
	case "tr":
		if table := b.currentTable(); table != nil {
			rowNumber := len(table.node.Children) + 1
			table.row = &artifact.Node{Kind: artifact.KindRow, Name: strconv.Itoa(rowNumber), Locator: artifact.Locator{Path: b.part, Subpath: table.node.Locator.Subpath + fmt.Sprintf("/row[%d]", rowNumber)}}
			return b.addNode()
		}
	case "tc":
		if table := b.currentTable(); table != nil && table.row != nil {
			column := len(table.row.Children) + 1
			row, _ := strconv.Atoi(table.row.Name)
			cell := spreadsheetCell(column, row)
			table.cell = &artifact.Node{Kind: artifact.KindCell, Name: cell, Locator: artifact.Locator{Path: b.part, Subpath: table.row.Locator.Subpath + fmt.Sprintf("/cell[%d]", column), Cell: cell}}
			return b.addNode()
		}
	case "p":
		b.paragraphs++
		b.paragraph = &wordParagraph{path: fmt.Sprintf("/document/paragraph[%d]", b.paragraphs)}
	case "pStyle":
		if b.paragraph != nil {
			b.paragraph.style = attr(element, "val")
		}
	case "hyperlink":
		if b.paragraph != nil {
			if rel, ok := b.relationships[relationshipID(element)]; ok {
				b.paragraph.links = append(b.paragraph.links, rel.Target)
			}
		}
	case "t":
		b.inText = true
	case "tab":
		if b.paragraph != nil {
			b.paragraph.text.WriteByte('\t')
		}
	case "br", "cr":
		if b.paragraph != nil {
			b.paragraph.text.WriteByte('\n')
		}
	}
	return nil
}

func (b *wordBuilder) end(element xml.EndElement) error {
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

func (b *wordBuilder) finishParagraph() error {
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
	locator := artifact.Locator{Path: b.part, Subpath: paragraph.path}
	node := artifact.Node{Kind: artifact.KindParagraph, Text: text, Locator: locator}
	if level := headingLevel(paragraph.style); level > 0 {
		node.Kind, node.Name, node.Text, node.Level = artifact.KindSection, text, "", level
	}
	if paragraph.style != "" || len(paragraph.links) > 0 {
		node.Attributes = make(map[string]string)
		if paragraph.style != "" {
			node.Attributes["style"] = paragraph.style
		}
		if len(paragraph.links) > 0 {
			node.Attributes["links"] = strings.Join(paragraph.links, " ")
		}
	}
	b.nodes = append(b.nodes, node)
	return b.addNode()
}

func (b *wordBuilder) currentTable() *wordTable {
	if len(b.tables) == 0 {
		return nil
	}
	return b.tables[len(b.tables)-1]
}

func (b *wordBuilder) addNode() error {
	b.nodeCount++
	if b.nodeCount > b.limits.MaxNodes {
		return &artifact.LimitError{Limit: "nodes", Value: int64(b.nodeCount), Max: int64(b.limits.MaxNodes)}
	}
	return nil
}

func wordPartLabel(part string) string {
	base := strings.TrimSuffix(path.Base(part), ".xml")
	switch {
	case strings.HasPrefix(base, "header"):
		return "Header"
	case strings.HasPrefix(base, "footer"):
		return "Footer"
	case base == "footnotes":
		return "Footnotes"
	case base == "endnotes":
		return "Endnotes"
	case base == "comments":
		return "Comments"
	default:
		return base
	}
}
