package parser

import (
	"context"
	"testing"

	"github.com/salman0ansari/artifactkit/artifact"
)

type stubParser struct {
	name  string
	score int
}

func (p stubParser) Name() string { return p.name }
func (p stubParser) Formats() []artifact.Format {
	return []artifact.Format{{Name: p.name, MediaType: "application/x-" + p.name}}
}
func (p stubParser) Probe(Source) Match { return Match{Score: p.score} }
func (p stubParser) Parse(context.Context, Source, artifact.Limits) (*artifact.Artifact, error) {
	return &artifact.Artifact{}, nil
}

func TestRegistryRanksDetectionsStably(t *testing.T) {
	registry := NewRegistry()
	for _, candidate := range []stubParser{{"fallback", 10}, {"first", 90}, {"second", 90}} {
		if err := registry.Register(candidate); err != nil {
			t.Fatal(err)
		}
	}
	detections := registry.Detect(Source{})
	if got := []string{detections[0].Parser.Name(), detections[1].Parser.Name(), detections[2].Parser.Name()}; got[0] != "first" || got[1] != "second" || got[2] != "fallback" {
		t.Fatalf("unexpected order: %v", got)
	}
}

func TestRegistryRejectsDuplicateName(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(stubParser{"same", 10}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(stubParser{"same", 20}); err == nil {
		t.Fatal("expected duplicate parser error")
	}
}
