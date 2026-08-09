package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/salman0ansari/artifactkit/artifact"
)

func TestMarkdownRendersTable(t *testing.T) {
	document := &artifact.Artifact{Name: "metrics.csv", Root: artifact.Node{Children: []artifact.Node{{
		Kind: artifact.KindTable,
		Children: []artifact.Node{
			{Kind: artifact.KindRow, Children: []artifact.Node{{Kind: artifact.KindCell, Text: "name"}, {Kind: artifact.KindCell, Text: "status"}}},
			{Kind: artifact.KindRow, Children: []artifact.Node{{Kind: artifact.KindCell, Text: "parser"}, {Kind: artifact.KindCell, Text: "ok"}}},
		},
	}}}}
	var output bytes.Buffer
	if err := Markdown(&output, document); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "| parser | ok |") {
		t.Fatalf("table not rendered:\n%s", output.String())
	}
}
