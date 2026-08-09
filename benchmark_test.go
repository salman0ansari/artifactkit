package artifactkit_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	artifactkit "github.com/salman0ansari/artifactkit"
	"github.com/salman0ansari/artifactkit/search"
)

func BenchmarkInspect(b *testing.B) {
	for _, name := range []string{"incident.md", "config.json", "metrics.csv", "brief.docx", "deck.pptx", "workbook.xlsx", "report.pdf"} {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			b.Fatal(err)
		}
		b.Run(name, func(b *testing.B) {
			engine := artifactkit.New()
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			for b.Loop() {
				if _, err := engine.InspectBytes(context.Background(), name, data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkSearch(b *testing.B) {
	data, err := os.ReadFile(filepath.Join("testdata", "brief.docx"))
	if err != nil {
		b.Fatal(err)
	}
	document, err := artifactkit.New().InspectBytes(context.Background(), "brief.docx", data)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if results := search.Find(document, "runbook links", 20); len(results) == 0 {
			b.Fatal("no results")
		}
	}
}
