package ziputil

import (
	"archive/zip"
	"bytes"
	"errors"
	"testing"

	"github.com/salman0ansari/artifactkit/artifact"
)

func TestOpenReadsNormalizedEntries(t *testing.T) {
	data := zipFixture(t, []zipItem{{"docs/readme.txt", "hello"}, {"data.json", "{}"}})
	reader, err := Open(data, artifact.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.Entries) != 2 || reader.Expanded != 7 || !reader.Has("docs/readme.txt") {
		t.Fatalf("unexpected ZIP inventory: %#v", reader)
	}
	content, err := reader.Read("docs/readme.txt")
	if err != nil || string(content) != "hello" {
		t.Fatalf("content = %q, err=%v", content, err)
	}
	if _, ok := reader.File("missing"); ok {
		t.Fatal("missing file was reported present")
	}
}

func TestOpenRejectsDuplicateNormalizedPathsAndLimits(t *testing.T) {
	duplicate := zipFixture(t, []zipItem{{"folder/../same.txt", "one"}, {"same.txt", "two"}})
	if _, err := Open(duplicate, artifact.DefaultLimits()); err == nil {
		t.Fatal("duplicate normalized path was accepted")
	}
	limits := artifact.DefaultLimits()
	limits.MaxArchiveEntries = 1
	if _, err := Open(zipFixture(t, []zipItem{{"a", "a"}, {"b", "b"}}), limits); err == nil {
		t.Fatal("entry limit was not enforced")
	}
	limits = artifact.DefaultLimits()
	limits.MaxCompressionRatio = 10
	var limitError *artifact.LimitError
	if err := CheckRatio(1_000, 1, limits); !errors.As(err, &limitError) {
		t.Fatalf("expected ratio limit error, got %v", err)
	}
}

func TestCleanPathRejectsTraversalAndAbsolutePaths(t *testing.T) {
	for _, value := range []string{"", "../secret", "/absolute", `C:\\secret`, "safe/../../secret", "bad\x00name"} {
		if _, err := CleanPath(value); err == nil {
			t.Errorf("CleanPath(%q) accepted an unsafe path", value)
		}
	}
	if got, err := CleanPath(`folder\\file.txt`); err != nil || got != "folder/file.txt" {
		t.Fatalf("normalized path = %q, err=%v", got, err)
	}
}

type zipItem struct{ name, content string }

func zipFixture(t *testing.T, items []zipItem) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, item := range items {
		entry, err := writer.Create(item.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(item.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
