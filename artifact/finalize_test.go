package artifact

import (
	"errors"
	"testing"
)

func TestFinalizeAssignsStableIDs(t *testing.T) {
	input := []byte("hello")
	first := &Artifact{Root: Node{Kind: KindDocument, Children: []Node{{Kind: KindParagraph, Text: "hello"}}}}
	second := &Artifact{Root: Node{Kind: KindDocument, Children: []Node{{Kind: KindParagraph, Text: "hello"}}}}
	if err := Finalize(first, input, DefaultLimits()); err != nil {
		t.Fatal(err)
	}
	if err := Finalize(second, input, DefaultLimits()); err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.Root.Children[0].ID != second.Root.Children[0].ID {
		t.Fatalf("IDs are not deterministic: %#v %#v", first, second)
	}
	if first.Root.ID == first.Root.Children[0].ID {
		t.Fatal("distinct nodes received the same ID")
	}
}

func TestFinalizeEnforcesNodeLimit(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxNodes = 1
	document := &Artifact{Root: Node{Kind: KindDocument, Children: []Node{{Kind: KindParagraph}}}}
	err := Finalize(document, nil, limits)
	var limitError *LimitError
	if !errors.As(err, &limitError) || limitError.Limit != "nodes" {
		t.Fatalf("expected node LimitError, got %v", err)
	}
}

func TestLocatorString(t *testing.T) {
	locator := Locator{Path: "slides/1.xml", Slide: 1, ByteRange: &ByteRange{Start: 10, End: 20}}
	if got, want := locator.String(), "slides/1.xml#slide:1#bytes:10-20"; got != want {
		t.Fatalf("Locator.String() = %q, want %q", got, want)
	}
}
