package search

import (
	"testing"

	"github.com/salman0ansari/artifactkit/artifact"
)

func TestFindRequiresEveryTokenAndRanksExactNames(t *testing.T) {
	document := &artifact.Artifact{Root: artifact.Node{Children: []artifact.Node{
		{ID: "a", Kind: artifact.KindSection, Name: "payment retry", Text: "worker failed once"},
		{ID: "b", Kind: artifact.KindParagraph, Text: "payment retry payment retry"},
		{ID: "c", Kind: artifact.KindParagraph, Text: "payment only"},
	}}}
	results := Find(document, "payment retry", 10)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].NodeID != "b" {
		t.Fatalf("stronger repeated match should rank first: %#v", results)
	}
}
