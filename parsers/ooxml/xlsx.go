package ooxml

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/salman0ansari/artifactkit/artifact"
)

type workbookSheet struct {
	Name string
	Part string
}

func (p *packageReader) parseXLSX(ctx context.Context) (*artifact.Artifact, error) {
	sheets, date1904, err := p.workbookSheets()
	if err != nil {
		return nil, err
	}
	shared, err := p.sharedStrings()
	if err != nil {
		return nil, err
	}
	result := &artifact.Artifact{
		Name: p.source.Name, Format: "xlsx",
		MediaType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		Metadata:  p.metadata(), Root: artifact.Node{Kind: artifact.KindDocument, Name: p.source.Name},
	}
	for _, sheet := range sheets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		node, err := p.parseWorksheet(ctx, sheet, shared)
		if err != nil {
			return nil, fmt.Errorf("parse XLSX sheet %q: %w", sheet.Name, err)
		}
		result.Root.Children = append(result.Root.Children, node)
	}
	p.packageResources(result, "xl/media/", "xl/embeddings/")
	if result.Metadata == nil {
		result.Metadata = make(map[string]string)
	}
	result.Metadata["sheets"] = strconv.Itoa(len(sheets))
	result.Metadata["date_system"] = "1900"
	if date1904 {
		result.Metadata["date_system"] = "1904"
	}
	return result, nil
}

func (p *packageReader) workbookSheets() ([]workbookSheet, bool, error) {
	relationships, err := p.relationships("xl/workbook.xml")
	if err != nil {
		return nil, false, err
	}
	data, err := p.archive.Read("xl/workbook.xml")
	if err != nil {
		return nil, false, err
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var sheets []workbookSheet
	date1904 := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return sheets, date1904, nil
		}
		if err != nil {
			return nil, false, err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "workbookPr":
			date1904 = attr(start, "date1904") == "1" || strings.EqualFold(attr(start, "date1904"), "true")
		case "sheet":
			rel, exists := relationships[relationshipID(start)]
			if !exists || rel.External || !p.archive.Has(rel.Target) {
				continue
			}
			sheets = append(sheets, workbookSheet{Name: attr(start, "name"), Part: rel.Target})
		}
	}
}

func (p *packageReader) sharedStrings() ([]string, error) {
	if !p.archive.Has("xl/sharedStrings.xml") {
		return nil, nil
	}
	data, err := p.archive.Read("xl/sharedStrings.xml")
	if err != nil {
		return nil, err
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var values []string
	var current strings.Builder
	inItem, inText, inPhonetic := false, false, false
	var textBytes int64
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return values, nil
		}
		if err != nil {
			return nil, fmt.Errorf("parse shared strings: %w", err)
		}
		switch typed := token.(type) {
		case xml.StartElement:
			switch typed.Name.Local {
			case "si":
				inItem = true
				current.Reset()
			case "rPh":
				inPhonetic = true
			case "t":
				inText = inItem && !inPhonetic
			}
		case xml.CharData:
			if inText {
				current.Write([]byte(typed))
			}
		case xml.EndElement:
			switch typed.Name.Local {
			case "t":
				inText = false
			case "rPh":
				inPhonetic = false
			case "si":
				value := current.String()
				textBytes += int64(len(value))
				if textBytes > p.limits.MaxTextBytes {
					return nil, &artifact.LimitError{Limit: "text bytes", Value: textBytes, Max: p.limits.MaxTextBytes}
				}
				values = append(values, value)
				if len(values) > p.limits.MaxNodes {
					return nil, &artifact.LimitError{Limit: "shared strings", Value: int64(len(values)), Max: int64(p.limits.MaxNodes)}
				}
				inItem = false
			}
		}
	}
}

type worksheetCell struct {
	ref, cellType, style       string
	value, formula, text       strings.Builder
	inValue, inFormula, inText bool
}

type worksheetBuilder struct {
	ctx           context.Context
	part          string
	sheet         string
	shared        []string
	relationships map[string]relationship
	limits        artifact.Limits
	table         artifact.Node
	row           *artifact.Node
	cell          *worksheetCell
	rowNumber     int
	nodeCount     int
	textBytes     int64
	hyperlinks    map[string]string
	merged        []string
}

func (p *packageReader) parseWorksheet(ctx context.Context, sheet workbookSheet, shared []string) (artifact.Node, error) {
	data, err := p.archive.Read(sheet.Part)
	if err != nil {
		return artifact.Node{}, err
	}
	relationships, err := p.relationships(sheet.Part)
	if err != nil {
		return artifact.Node{}, err
	}
	builder := worksheetBuilder{
		ctx: ctx, part: sheet.Part, sheet: sheet.Name, shared: shared, relationships: relationships, limits: p.limits,
		table:      artifact.Node{Kind: artifact.KindTable, Name: sheet.Name, Locator: artifact.Locator{Path: sheet.Part, Subpath: "/worksheet/sheetData", Sheet: sheet.Name}},
		hyperlinks: make(map[string]string), nodeCount: 2,
	}
	if err := builder.parse(data); err != nil {
		return artifact.Node{}, err
	}
	if len(builder.merged) > 0 {
		if builder.table.Attributes == nil {
			builder.table.Attributes = make(map[string]string)
		}
		builder.table.Attributes["merged_ranges"] = strings.Join(builder.merged, " ")
	}
	for rowIndex := range builder.table.Children {
		for cellIndex := range builder.table.Children[rowIndex].Children {
			cell := &builder.table.Children[rowIndex].Children[cellIndex]
			if target := builder.hyperlinks[cell.Name]; target != "" {
				if cell.Attributes == nil {
					cell.Attributes = make(map[string]string)
				}
				cell.Attributes["hyperlink"] = target
			}
		}
	}
	return artifact.Node{
		Kind: artifact.KindSheet, Name: sheet.Name, Locator: artifact.Locator{Path: sheet.Part, Sheet: sheet.Name},
		Children: []artifact.Node{builder.table},
	}, nil
}

func (b *worksheetBuilder) parse(data []byte) error {
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
			if b.cell != nil {
				switch {
				case b.cell.inValue:
					b.cell.value.Write([]byte(typed))
				case b.cell.inFormula:
					b.cell.formula.Write([]byte(typed))
				case b.cell.inText:
					b.cell.text.Write([]byte(typed))
				}
			}
		case xml.EndElement:
			if err := b.end(typed); err != nil {
				return err
			}
		}
	}
}

func (b *worksheetBuilder) start(element xml.StartElement) error {
	switch element.Name.Local {
	case "row":
		b.rowNumber++
		if value, err := strconv.Atoi(attr(element, "r")); err == nil && value > 0 {
			b.rowNumber = value
		}
		b.row = &artifact.Node{Kind: artifact.KindRow, Name: strconv.Itoa(b.rowNumber), Locator: artifact.Locator{Path: b.part, Subpath: fmt.Sprintf("/worksheet/sheetData/row[%d]", b.rowNumber), Sheet: b.sheet}}
		return b.addNode()
	case "c":
		if b.row == nil {
			return nil
		}
		cellRef := attr(element, "r")
		if cellRef == "" {
			cellRef = spreadsheetCell(len(b.row.Children)+1, b.rowNumber)
		}
		b.cell = &worksheetCell{ref: cellRef, cellType: attr(element, "t"), style: attr(element, "s")}
	case "v":
		if b.cell != nil {
			b.cell.inValue = true
		}
	case "f":
		if b.cell != nil {
			b.cell.inFormula = true
		}
	case "t":
		if b.cell != nil {
			b.cell.inText = true
		}
	case "hyperlink":
		cellRef := attr(element, "ref")
		if rel, ok := b.relationships[relationshipID(element)]; ok {
			b.hyperlinks[cellRef] = rel.Target
		} else if location := attr(element, "location"); location != "" {
			b.hyperlinks[cellRef] = "#" + location
		}
	case "mergeCell":
		if value := attr(element, "ref"); value != "" {
			b.merged = append(b.merged, value)
		}
	}
	return nil
}

func (b *worksheetBuilder) end(element xml.EndElement) error {
	switch element.Name.Local {
	case "v":
		if b.cell != nil {
			b.cell.inValue = false
		}
	case "f":
		if b.cell != nil {
			b.cell.inFormula = false
		}
	case "t":
		if b.cell != nil {
			b.cell.inText = false
		}
	case "c":
		return b.finishCell()
	case "row":
		if b.row != nil {
			b.table.Children = append(b.table.Children, *b.row)
			b.row = nil
		}
	}
	return nil
}

func (b *worksheetBuilder) finishCell() error {
	if b.cell == nil || b.row == nil {
		b.cell = nil
		return nil
	}
	state := b.cell
	b.cell = nil
	value := state.value.String()
	switch state.cellType {
	case "s":
		index, err := strconv.Atoi(value)
		if err != nil || index < 0 || index >= len(b.shared) {
			return fmt.Errorf("cell %s has invalid shared string index %q", state.ref, value)
		}
		value = b.shared[index]
	case "inlineStr":
		value = state.text.String()
	case "b":
		if value == "1" {
			value = "true"
		} else if value == "0" {
			value = "false"
		}
	}
	attributes := make(map[string]string)
	if formula := strings.TrimSpace(state.formula.String()); formula != "" {
		attributes["formula"] = formula
	}
	if state.cellType != "" {
		attributes["type"] = state.cellType
	}
	if state.style != "" {
		attributes["style"] = state.style
	}
	if len(attributes) == 0 {
		attributes = nil
	}
	cell := artifact.Node{
		Kind: artifact.KindCell, Name: state.ref, Text: value, Attributes: attributes,
		Locator: artifact.Locator{Path: b.part, Subpath: fmt.Sprintf("/worksheet/sheetData/row[%d]/cell[%s]", b.rowNumber, state.ref), Sheet: b.sheet, Cell: state.ref},
	}
	b.row.Children = append(b.row.Children, cell)
	b.textBytes += int64(len(value))
	if b.textBytes > b.limits.MaxTextBytes {
		return &artifact.LimitError{Limit: "text bytes", Value: b.textBytes, Max: b.limits.MaxTextBytes}
	}
	return b.addNode()
}

func (b *worksheetBuilder) addNode() error {
	b.nodeCount++
	if b.nodeCount > b.limits.MaxNodes {
		return &artifact.LimitError{Limit: "nodes", Value: int64(b.nodeCount), Max: int64(b.limits.MaxNodes)}
	}
	return nil
}
