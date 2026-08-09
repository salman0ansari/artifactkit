package email

import (
	"context"
	"os"
	"testing"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
)

func TestEmailParserReadsBodyAndAttachment(t *testing.T) {
	data, err := os.ReadFile("../../testdata/message.eml")
	if err != nil {
		t.Fatal(err)
	}
	document, err := New().Parse(context.Background(), parser.NewSource("message.eml", "", data), artifact.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if document.Metadata["subject"] != "ArtifactKit checkpoint" {
		t.Fatalf("subject = %q", document.Metadata["subject"])
	}
	if len(document.Resources) != 1 || document.Resources[0].Name != "config.json" || document.Resources[0].Size == 0 {
		t.Fatalf("unexpected resources: %#v", document.Resources)
	}
	if len(document.Root.Children) != 2 || document.Root.Children[0].Kind != artifact.KindParagraph || document.Root.Children[1].Kind != artifact.KindAttachment {
		t.Fatalf("unexpected email nodes: %#v", document.Root.Children)
	}
}
