package archive

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
)

func TestArchiveParserInventoriesRealFiles(t *testing.T) {
	for _, test := range []struct {
		name, format string
		resources    int
	}{{"sample.zip", "zip", 2}, {"sample.tar.gz", "tar.gz", 1}, {"report.txt.gz", "gzip", 1}} {
		t.Run(test.name, func(t *testing.T) {
			data, err := os.ReadFile("../../testdata/" + test.name)
			if err != nil {
				t.Fatal(err)
			}
			document, err := New().Parse(context.Background(), parser.NewSource(test.name, "", data), artifact.DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			if document.Format != test.format || len(document.Resources) != test.resources {
				t.Fatalf("unexpected archive: format=%s resources=%#v", document.Format, document.Resources)
			}
		})
	}
}

func TestZIPRejectsTraversal(t *testing.T) {
	data := makeZIP(t, map[string]string{"../escape.txt": "no"})
	_, err := New().Parse(context.Background(), parser.NewSource("bad.zip", "", data), artifact.DefaultLimits())
	if err == nil {
		t.Fatal("expected unsafe path error")
	}
}

func TestZIPEnforcesEntryAndCompressionLimits(t *testing.T) {
	data := makeZIP(t, map[string]string{"one.txt": "1", "two.txt": "2"})
	limits := artifact.DefaultLimits()
	limits.MaxArchiveEntries = 1
	_, err := New().Parse(context.Background(), parser.NewSource("many.zip", "", data), limits)
	var limitError *artifact.LimitError
	if !errors.As(err, &limitError) || limitError.Limit != "archive entries" {
		t.Fatalf("expected archive entry limit, got %v", err)
	}

	data = makeZIP(t, map[string]string{"bomb.txt": string(bytes.Repeat([]byte("a"), 100_000))})
	limits = artifact.DefaultLimits()
	limits.MaxCompressionRatio = 5
	_, err = New().Parse(context.Background(), parser.NewSource("ratio.zip", "", data), limits)
	if !errors.As(err, &limitError) || limitError.Limit != "compression ratio" {
		t.Fatalf("expected compression ratio limit, got %v", err)
	}
}

func makeZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
