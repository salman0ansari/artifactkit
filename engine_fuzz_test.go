package artifactkit_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	artifactkit "github.com/salman0ansari/artifactkit"
	"github.com/salman0ansari/artifactkit/artifact"
)

func FuzzEngineInspect(f *testing.F) {
	for _, name := range []string{"incident.md", "config.json", "settings.yaml", "pipeline.toml", "metrics.csv", "page.html", "message.eml", "sample.zip", "brief.docx", "deck.pptx", "workbook.xlsx", "report.pdf"} {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(name, data)
	}
	limits := artifact.DefaultLimits()
	limits.MaxInputBytes = 1 << 20
	limits.MaxExpandedBytes = 4 << 20
	limits.MaxStoredBytes = 4 << 20
	limits.MaxNodes = 20_000
	engine := artifactkit.New(artifactkit.WithLimits(limits), artifactkit.WithResourceStore(nil))
	f.Fuzz(func(t *testing.T, name string, data []byte) {
		if len(data) > int(limits.MaxInputBytes) {
			t.Skip()
		}
		_, _ = engine.InspectBytes(context.Background(), name, data)
	})
}
