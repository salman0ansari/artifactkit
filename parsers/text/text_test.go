package text

import (
	"context"
	"errors"
	"testing"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
)

func TestParseMarkdownPreservesSectionsParagraphsAndRanges(t *testing.T) {
	data := []byte("# Title\n\nfirst\nline\n\n## Next\nbody\n")
	document, err := New().Parse(context.Background(), parser.NewSource("note.md", "", data), artifact.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if document.Format != "markdown" || document.Metadata["title"] != "Title" || len(document.Root.Children) != 2 {
		t.Fatalf("unexpected Markdown: %#v", document)
	}
	first := document.Root.Children[0]
	if first.Level != 1 || len(first.Children) != 1 || first.Children[0].Text != "first\nline" || first.Children[0].Locator.ByteRange == nil {
		t.Fatalf("unexpected first section: %#v", first)
	}
}

func TestParsePlainTextHandlesCRLFAndProbeRejectsBinary(t *testing.T) {
	document, err := New().Parse(context.Background(), parser.NewSource("note.txt", "", []byte("one\r\nline\r\n\r\ntwo\r\n")), artifact.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Root.Children) != 2 || document.Root.Children[0].Text != "one\nline" || document.Root.Children[1].Text != "two" {
		t.Fatalf("unexpected paragraphs: %#v", document.Root.Children)
	}
	if New().Probe(parser.NewSource("binary.txt", "", []byte{'a', 0, 'b'})).Score != 0 {
		t.Fatal("NUL-containing input was accepted as text")
	}
	if New().Probe(parser.NewSource("invalid.txt", "", []byte{0xff})).Score != 0 {
		t.Fatal("invalid UTF-8 was accepted as text")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New().Parse(ctx, parser.NewSource("note.txt", "", []byte("value")), artifact.DefaultLimits()); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
