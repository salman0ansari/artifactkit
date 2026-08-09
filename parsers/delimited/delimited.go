// Package delimited parses CSV and TSV tables.
package delimited

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
)

type Parser struct{}

func New() *Parser           { return &Parser{} }
func (*Parser) Name() string { return "delimited" }

func (*Parser) Formats() []artifact.Format {
	return []artifact.Format{
		{Name: "csv", MediaType: "text/csv", Extensions: []string{".csv"}},
		{Name: "tsv", MediaType: "text/tab-separated-values", Extensions: []string{".tsv"}},
	}
}

func (*Parser) Probe(source parser.Source) parser.Match {
	switch source.Extension() {
	case ".csv":
		return parser.Match{Score: 90, Reason: "CSV extension"}
	case ".tsv":
		return parser.Match{Score: 90, Reason: "TSV extension"}
	}
	if bytes.Count(source.Data, []byte{'\n'}) > 0 && bytes.Count(source.Data, []byte{','}) > 1 {
		return parser.Match{Score: 35, Reason: "repeated delimiters across lines"}
	}
	return parser.Match{}
}

func (*Parser) Parse(ctx context.Context, source parser.Source, limits artifact.Limits) (*artifact.Artifact, error) {
	format := "csv"
	mediaType := "text/csv"
	delimiter := ','
	if source.Extension() == ".tsv" {
		format = "tsv"
		mediaType = "text/tab-separated-values"
		delimiter = '\t'
	}

	reader := csv.NewReader(bytes.NewReader(source.Data))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	table := artifact.Node{Kind: artifact.KindTable, Name: source.Name}
	var previousOffset int64
	rowNumber := 0
	nodeCount := 2
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", strings.ToUpper(format), err)
		}
		rowNumber++
		offset := reader.InputOffset()
		row := artifact.Node{
			Kind: artifact.KindRow, Name: strconv.Itoa(rowNumber),
			Locator: artifact.Locator{ByteRange: &artifact.ByteRange{Start: previousOffset, End: offset}},
		}
		if rowNumber == 1 {
			row.Attributes = map[string]string{"role": "header"}
		}
		for column, value := range record {
			cell := cellName(column+1, rowNumber)
			row.Children = append(row.Children, artifact.Node{
				Kind: artifact.KindCell, Name: cell, Text: value,
				Locator: artifact.Locator{Cell: cell, ByteRange: &artifact.ByteRange{Start: previousOffset, End: offset}},
			})
		}
		table.Children = append(table.Children, row)
		previousOffset = offset
		nodeCount += 1 + len(row.Children)
		if nodeCount > limits.MaxNodes {
			return nil, &artifact.LimitError{Limit: "nodes", Value: int64(nodeCount), Max: int64(limits.MaxNodes)}
		}
	}

	return &artifact.Artifact{
		Name: source.Name, Format: format, MediaType: mediaType,
		Root:     artifact.Node{Kind: artifact.KindDocument, Name: source.Name, Children: []artifact.Node{table}},
		Metadata: map[string]string{"rows": strconv.Itoa(len(table.Children))},
	}, nil
}

func cellName(column, row int) string {
	var letters []byte
	for column > 0 {
		column--
		letters = append([]byte{byte('A' + column%26)}, letters...)
		column /= 26
	}
	return string(letters) + strconv.Itoa(row)
}
