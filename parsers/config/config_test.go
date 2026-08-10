package config

import (
	"context"
	"errors"
	"testing"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
)

func TestYAMLTypedTreeAndPositions(t *testing.T) {
	source := parser.NewSource("settings.yaml", "", []byte("agent:\n  name: researcher\n  enabled: true\n  tags: [local, safe]\n"))
	document, err := New().Parse(context.Background(), source, artifact.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	name := findValue(&document.Root, "/agent/name")
	enabled := findValue(&document.Root, "/agent/enabled")
	if name == nil || name.Text != "researcher" || name.Attributes["line"] != "2" {
		t.Fatalf("unexpected YAML name: %#v", name)
	}
	if enabled == nil || enabled.Attributes["type"] != "boolean" {
		t.Fatalf("unexpected YAML boolean: %#v", enabled)
	}
}

func TestYAMLRejectsAliasCyclesAndDepthOverflow(t *testing.T) {
	limits := artifact.DefaultLimits()
	limits.MaxNestingDepth = 2
	_, err := New().Parse(context.Background(), parser.NewSource("deep.yaml", "", []byte("a:\n  b:\n    c: value\n")), limits)
	var limitError *artifact.LimitError
	if !errors.As(err, &limitError) {
		t.Fatalf("expected depth error, got %v", err)
	}
	_, err = New().Parse(context.Background(), parser.NewSource("cycle.yaml", "", []byte("value: &value [*value]\n")), artifact.DefaultLimits())
	if err == nil {
		t.Fatal("expected alias cycle error")
	}
}

func TestTOMLTypedTreeAndDeterministicKeys(t *testing.T) {
	source := parser.NewSource("pipeline.toml", "", []byte("title = \"ArtifactKit\"\n[agent]\nname = \"researcher\"\nenabled = true\nretries = 3\n"))
	document, err := New().Parse(context.Background(), source, artifact.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if got := findValue(&document.Root, "/agent/retries"); got == nil || got.Text != "3" || got.Attributes["type"] != "number" {
		t.Fatalf("unexpected TOML number: %#v", got)
	}
	root := document.Root.Children[0]
	if len(root.Children) != 2 || root.Children[0].Name != "agent" || root.Children[1].Name != "title" {
		t.Fatalf("keys are not sorted: %#v", root.Children)
	}
}

func findValue(root *artifact.Node, path string) *artifact.Node {
	var found *artifact.Node
	artifact.Walk(root, func(node *artifact.Node) bool {
		if node.Kind == artifact.KindValue && node.Locator.Path == path {
			found = node
			return false
		}
		return found == nil
	})
	return found
}
