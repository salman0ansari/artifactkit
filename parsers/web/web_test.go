package web

import (
	"context"
	"os"
	"testing"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
)

func TestHTMLParserExtractsSemanticsAndSkipsScripts(t *testing.T) {
	data, err := os.ReadFile("../../testdata/page.html")
	if err != nil {
		t.Fatal(err)
	}
	document, err := NewHTML().Parse(context.Background(), parser.NewSource("page.html", "", data), artifact.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if document.Metadata["title"] != "Agent Operations" {
		t.Fatalf("title = %q", document.Metadata["title"])
	}
	var table, image, scriptText bool
	artifact.Walk(&document.Root, func(node *artifact.Node) bool {
		table = table || node.Kind == artifact.KindTable
		image = image || (node.Kind == artifact.KindImage && node.Attributes["src"] == "diagram.png")
		scriptText = scriptText || node.Text == `ignoreSecret("never index this")`
		return true
	})
	if !table || !image || scriptText {
		t.Fatalf("unexpected HTML tree: table=%v image=%v script=%v", table, image, scriptText)
	}
}

func TestXMLParserPreservesElementPaths(t *testing.T) {
	data, err := os.ReadFile("../../testdata/catalog.xml")
	if err != nil {
		t.Fatal(err)
	}
	document, err := NewXML().Parse(context.Background(), parser.NewSource("catalog.xml", "", data), artifact.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	second := document.Root.Children[0].Children[1]
	if got, want := second.Locator.Path, "/catalog[1]/artifact[2]"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}
