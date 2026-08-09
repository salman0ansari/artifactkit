package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/salman0ansari/artifactkit/artifact"
)

func TestRunInspectJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	path := filepath.Join("..", "..", "testdata", "incident.md")
	code := Run(context.Background(), []string{"inspect", "--pretty", path}, bytes.NewReader(nil), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	var document artifact.Artifact
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if document.Format != "markdown" {
		t.Fatalf("format = %q", document.Format)
	}
}

func TestRunFindFromStdin(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"find", "--name", "data.json", "-", "researcher"},
		bytes.NewBufferString(`{"name":"researcher"}`), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"path": "/name"`)) {
		t.Fatalf("missing JSON Pointer: %s", stdout.String())
	}
}
