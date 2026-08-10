package artifactkit_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	artifactkit "github.com/salman0ansari/artifactkit"
	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
	"github.com/salman0ansari/artifactkit/search"
)

func TestEngineInspectsRealFiles(t *testing.T) {
	tests := []struct {
		file, format string
		kind         artifact.Kind
	}{
		{"incident.md", "markdown", artifact.KindSection},
		{"config.json", "json", artifact.KindObject},
		{"metrics.csv", "csv", artifact.KindTable},
		{"page.html", "html", artifact.KindSection},
		{"catalog.xml", "xml", artifact.KindElement},
		{"message.eml", "eml", artifact.KindParagraph},
		{"pixel.png", "png", artifact.KindImage},
		{"sample.zip", "zip", artifact.KindArchive},
		{"sample.tar.gz", "tar.gz", artifact.KindArchive},
		{"report.txt.gz", "gzip", artifact.KindArchive},
		{"brief.docx", "docx", artifact.KindSection},
		{"deck.pptx", "pptx", artifact.KindSlide},
		{"workbook.xlsx", "xlsx", artifact.KindSheet},
		{"report.pdf", "pdf", artifact.KindPage},
	}
	engine := artifactkit.New()
	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			document, err := engine.InspectPath(context.Background(), filepath.Join("testdata", test.file))
			if err != nil {
				t.Fatal(err)
			}
			if document.Format != test.format {
				t.Fatalf("format = %q, want %q", document.Format, test.format)
			}
			if len(document.Root.Children) == 0 || document.Root.Children[0].Kind != test.kind {
				t.Fatalf("unexpected root children: %#v", document.Root.Children)
			}
			if document.ID == "" || document.Root.ID == "" {
				t.Fatal("finalized IDs are empty")
			}
		})
	}
}

func TestEngineInspectsLazyResourceAsArtifact(t *testing.T) {
	engine := artifactkit.New()
	container, err := engine.InspectPath(context.Background(), filepath.Join("testdata", "sample.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if len(container.Resources) == 0 {
		t.Fatal("archive exposed no resources")
	}
	document, err := engine.InspectResource(context.Background(), container.Resources[0].URI)
	if err != nil {
		t.Fatal(err)
	}
	if document.Format != "text" || document.Name != "docs/readme.txt" {
		t.Fatalf("unexpected nested artifact: %#v", document)
	}
}

func TestEnginePreservesProvenance(t *testing.T) {
	engine := artifactkit.New()
	document, err := engine.InspectPath(context.Background(), filepath.Join("testdata", "metrics.csv"))
	if err != nil {
		t.Fatal(err)
	}
	cell := document.Root.Children[0].Children[1].Children[1]
	if cell.Locator.Cell != "B2" || cell.Locator.ByteRange == nil {
		t.Fatalf("unexpected cell provenance: %#v", cell.Locator)
	}

	document, err = engine.InspectPath(context.Background(), filepath.Join("testdata", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	results := search.Find(document, "researcher", 10)
	if len(results) != 1 || results[0].Locator.Path != "/agent/name" {
		t.Fatalf("unexpected JSON search result: %#v", results)
	}
}

func TestEngineRejectsOversizedAndUnsupportedInputs(t *testing.T) {
	limits := artifact.DefaultLimits()
	limits.MaxInputBytes = 3
	engine := artifactkit.New(artifactkit.WithLimits(limits))
	_, err := engine.InspectBytes(context.Background(), "large.txt", []byte("four"))
	var limitError *artifact.LimitError
	if !errors.As(err, &limitError) {
		t.Fatalf("expected LimitError, got %v", err)
	}

	engine = artifactkit.New()
	_, err = engine.InspectBytes(context.Background(), "blob.bin", []byte{0, 1, 2, 3})
	if !errors.Is(err, parser.ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
}

func TestEngineHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := artifactkit.New().InspectBytes(ctx, "note.txt", []byte("hello"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
