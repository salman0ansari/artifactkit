package main

import (
	"bytes"
	"context"
	"testing"
)

func TestRunWiresCLIStreamsAndArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"version"}, bytes.NewReader(nil), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, stderr.String())
	}
	if stdout.String() == "" || stderr.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
