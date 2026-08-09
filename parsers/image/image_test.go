package imageinfo

import (
	"context"
	"os"
	"testing"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
)

func TestImageParserReadsConfigurationOnly(t *testing.T) {
	data, err := os.ReadFile("../../testdata/pixel.png")
	if err != nil {
		t.Fatal(err)
	}
	document, err := New().Parse(context.Background(), parser.NewSource("pixel.png", "", data), artifact.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if document.Format != "png" || document.Metadata["width"] != "2" || document.Metadata["height"] != "3" {
		t.Fatalf("unexpected image metadata: %#v", document)
	}
}
