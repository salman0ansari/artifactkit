package pdfparser

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
)

func TestPDFExtractsPageScopedText(t *testing.T) {
	data, err := os.ReadFile("../../testdata/report.pdf")
	if err != nil {
		t.Fatal(err)
	}
	document, err := New().Parse(context.Background(), parser.NewSource("report.pdf", "", data), artifact.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if document.Metadata["pages"] != "1" || len(document.Root.Children) != 1 {
		t.Fatalf("unexpected PDF: %#v", document)
	}
	page := document.Root.Children[0]
	if page.Kind != artifact.KindPage || page.Locator.Page != 1 || !strings.Contains(page.Text, "ArtifactKit PDF fixture") {
		t.Fatalf("unexpected PDF page: %#v", page)
	}
}

func TestPDFMalformedInputReturnsErrorInsteadOfPanicking(t *testing.T) {
	_, err := New().Parse(context.Background(), parser.NewSource("broken.pdf", "", []byte("%PDF-1.4")), artifact.DefaultLimits())
	if err == nil {
		t.Fatal("expected malformed PDF error")
	}
}
