package resource_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	artifactkit "github.com/salman0ansari/artifactkit"
	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/resource"
)

func TestEngineReadsLazyResourcesAcrossContainers(t *testing.T) {
	tests := []struct {
		file string
		want []byte
	}{
		{"sample.zip", []byte("ArtifactKit archive fixture\n")},
		{"sample.tar.gz", []byte("service,status\nparser,ready\n")},
		{"report.txt.gz", []byte("bounded gzip fixture\n")},
		{"message.eml", []byte(`{"safe":true,"tool":"inspect"}`)},
		{"brief.docx", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}},
	}
	engine := artifactkit.New()
	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			document, err := engine.InspectPath(context.Background(), filepath.Join("..", "testdata", test.file))
			if err != nil {
				t.Fatal(err)
			}
			if len(document.Resources) == 0 {
				t.Fatal("no lazy resources")
			}
			item := document.Resources[0]
			limit := item.Size
			if int64(len(test.want)) < limit {
				limit = int64(len(test.want))
			}
			content, err := engine.ReadResource(context.Background(), item.URI, 0, limit)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(content.Data, test.want[:limit]) {
				t.Fatalf("content = %q, want prefix %q", content.Data, test.want[:limit])
			}
		})
	}
}

func TestResourceReadSupportsRanges(t *testing.T) {
	engine := artifactkit.New()
	document, err := engine.InspectPath(context.Background(), filepath.Join("..", "testdata", "sample.zip"))
	if err != nil {
		t.Fatal(err)
	}
	content, err := engine.ReadResource(context.Background(), document.Resources[0].URI, 12, 7)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(content.Data), "archive"; got != want || content.EOF {
		t.Fatalf("range = %q eof=%v, want %q eof=false", got, content.EOF, want)
	}
}

func TestResourceReadAllUsesInspectionLimit(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "testdata", "sample.zip"))
	if err != nil {
		t.Fatal(err)
	}
	engine := artifactkit.New()
	document, err := engine.InspectBytes(context.Background(), "sample.zip", data)
	if err != nil {
		t.Fatal(err)
	}
	item := document.Resources[0]
	store := resource.NewStore(artifact.DefaultLimits())
	if !store.Put(document, data) {
		t.Fatal("store rejected fixture")
	}
	if _, err := store.ReadAll(context.Background(), item.URI, item.Size-1); err == nil {
		t.Fatal("expected inspection limit error")
	}
	content, err := store.ReadAll(context.Background(), item.URI, item.Size)
	if err != nil || string(content.Data) != "ArtifactKit archive fixture\n" {
		t.Fatalf("unexpected content: %#v err=%v", content, err)
	}
}

func TestResourceStoreEvictsLeastRecentlyUsedSource(t *testing.T) {
	limits := artifact.DefaultLimits()
	limits.MaxStoredBytes = 8
	limits.MaxStoredArtifacts = 1
	store := resource.NewStore(limits)
	first := storedFixture(t, []byte("one1"), limits)
	second := storedFixture(t, []byte("two2"), limits)
	if !store.Put(first.document, first.data) || !store.Put(second.document, second.data) {
		t.Fatal("store rejected fixtures")
	}
	if stats := store.Stats(); stats.Artifacts != 1 || stats.Bytes != 4 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
	_, err := store.Read(context.Background(), first.document.Resources[0].URI, 0, 1)
	if !errors.Is(err, resource.ErrExpired) {
		t.Fatalf("expected ErrExpired, got %v", err)
	}
}

func TestResourceStoreCanBeDisabled(t *testing.T) {
	engine := artifactkit.New(artifactkit.WithResourceStore(nil))
	document, err := engine.InspectPath(context.Background(), filepath.Join("..", "testdata", "sample.zip"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.ReadResource(context.Background(), document.Resources[0].URI, 0, 1)
	if !errors.Is(err, resource.ErrExpired) {
		t.Fatalf("expected ErrExpired, got %v", err)
	}
}

type fixture struct {
	document *artifact.Artifact
	data     []byte
}

func storedFixture(t *testing.T, data []byte, limits artifact.Limits) fixture {
	t.Helper()
	uri := artifact.ResourceURI(data, "entry", "value.txt")
	document := &artifact.Artifact{
		Name: "fixture.zip", Format: "zip", MediaType: "application/zip",
		Root:      artifact.Node{Kind: artifact.KindDocument},
		Resources: []artifact.Resource{{URI: uri, Name: "value.txt", Size: int64(len(data)), Locator: artifact.Locator{Path: "value.txt"}}},
	}
	if err := artifact.Finalize(document, data, limits); err != nil {
		t.Fatal(err)
	}
	return fixture{document: document, data: data}
}
