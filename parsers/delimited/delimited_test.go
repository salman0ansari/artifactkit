package delimited

import (
	"context"
	"errors"
	"testing"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
)

func TestParseCSVPreservesQuotedCellsAndRanges(t *testing.T) {
	source := parser.NewSource("agents.csv", "", []byte("name,note\r\nagent,\"fast, safe\"\r\n"))
	document, err := New().Parse(context.Background(), source, artifact.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	table := document.Root.Children[0]
	if document.Format != "csv" || document.Metadata["rows"] != "2" || len(table.Children) != 2 {
		t.Fatalf("unexpected CSV: %#v", document)
	}
	cell := table.Children[1].Children[1]
	if cell.Name != "B2" || cell.Text != "fast, safe" || cell.Locator.ByteRange == nil || cell.Locator.ByteRange.Start == 0 {
		t.Fatalf("unexpected quoted cell: %#v", cell)
	}
	if table.Children[0].Attributes["role"] != "header" || cellName(27, 3) != "AA3" {
		t.Fatal("header role or spreadsheet cell naming is incorrect")
	}
}

func TestParseTSVAndRejectMalformedOrOversizedTables(t *testing.T) {
	document, err := New().Parse(context.Background(), parser.NewSource("agents.tsv", "", []byte("name\tready\nagent\ttrue\n")), artifact.DefaultLimits())
	if err != nil || document.Format != "tsv" || document.Root.Children[0].Children[1].Children[1].Text != "true" {
		t.Fatalf("unexpected TSV: %#v err=%v", document, err)
	}
	if _, err := New().Parse(context.Background(), parser.NewSource("bad.csv", "", []byte("name\n\"unterminated")), artifact.DefaultLimits()); err == nil {
		t.Fatal("malformed CSV was accepted")
	}
	limits := artifact.DefaultLimits()
	limits.MaxNodes = 4
	_, err = New().Parse(context.Background(), parser.NewSource("large.csv", "", []byte("a,b\n1,2\n")), limits)
	var limitError *artifact.LimitError
	if !errors.As(err, &limitError) {
		t.Fatalf("expected node limit error, got %v", err)
	}
}
