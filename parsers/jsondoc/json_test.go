package jsondoc

import (
	"context"
	"errors"
	"testing"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
)

func TestParseJSONBuildsSortedTypedPointerTree(t *testing.T) {
	source := parser.NewSource("value.json", "", []byte(`{"z":null,"a/b":{"~key":[true,1,"x"]}}`))
	document, err := New().Parse(context.Background(), source, artifact.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	root := document.Root.Children[0]
	if root.Children[0].Name != "a/b" || root.Children[1].Name != "z" {
		t.Fatalf("object keys are not deterministic: %#v", root.Children)
	}
	value := findJSONValue(&document.Root, "/a~1b/~0key/1")
	if value == nil || value.Text != "1" || value.Attributes["type"] != "number" {
		t.Fatalf("unexpected typed value: %#v", value)
	}
	if New().Probe(source).Score != 100 {
		t.Fatal("valid JSON did not receive maximum confidence")
	}
}

func TestParseJSONRejectsTrailingValuesAndLimits(t *testing.T) {
	if _, err := New().Parse(context.Background(), parser.NewSource("bad.json", "", []byte(`{} {}`)), artifact.DefaultLimits()); err == nil {
		t.Fatal("multiple top-level values were accepted")
	}
	limits := artifact.DefaultLimits()
	limits.MaxNestingDepth = 2
	_, err := New().Parse(context.Background(), parser.NewSource("deep.json", "", []byte(`{"a":{"b":1}}`)), limits)
	var limitError *artifact.LimitError
	if !errors.As(err, &limitError) {
		t.Fatalf("expected depth error, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New().Parse(ctx, parser.NewSource("value.json", "", []byte(`{}`)), artifact.DefaultLimits()); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func findJSONValue(root *artifact.Node, path string) *artifact.Node {
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
