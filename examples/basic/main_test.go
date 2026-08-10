package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunInspectsAndSearchesRealFixture(t *testing.T) {
	var output bytes.Buffer
	if err := run(context.Background(), filepath.Join("..", "..", "testdata", "metrics.csv"), &output); err != nil {
		t.Fatal(err)
	}
	if text := output.String(); !strings.Contains(text, ": csv (") || !strings.Contains(text, "cell:C1") || !strings.Contains(text, "status") {
		t.Fatalf("unexpected example output: %s", text)
	}
}
