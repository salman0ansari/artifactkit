package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	artifactkit "github.com/salman0ansari/artifactkit"
	"github.com/salman0ansari/artifactkit/artifact"
)

func TestServiceAgentWorkflow(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(artifactkit.New(), Options{Roots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	document, err := service.Inspect(context.Background(), filepath.Join(root, "workbook.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	summary := Summarize(document)
	if summary.Format != "xlsx" || summary.Nodes == 0 || len(summary.Outline) == 0 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	results, err := service.Find(context.Background(), document.ID, "SUM(1,2)", 10)
	if err != nil || len(results) != 1 || results[0].Name != "C2" {
		t.Fatalf("unexpected formula search: results=%#v err=%v", results, err)
	}
	table, err := service.ExtractTable(context.Background(), document.ID, "Agents", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Columns) != 3 || len(table.Rows) != 1 || table.Rows[0][2].Value != "3" {
		t.Fatalf("unexpected table: %#v", table)
	}
	node, err := service.ReadNode(context.Background(), document.ID, results[0].NodeID, 0, 1)
	if err != nil || node.Text != "3" {
		t.Fatalf("unexpected node read: node=%#v err=%v", node, err)
	}
	provenance, err := service.Provenance(context.Background(), document.ID, results[0].NodeID)
	if err != nil || provenance.Locator.Cell != "C2" || len(provenance.Trail) < 4 {
		t.Fatalf("unexpected provenance: value=%#v err=%v", provenance, err)
	}
}

func TestServiceInspectsNestedResource(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(artifactkit.New(), Options{Roots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	container, err := service.Inspect(context.Background(), filepath.Join(root, "sample.zip"))
	if err != nil {
		t.Fatal(err)
	}
	nested, err := service.InspectResource(context.Background(), container.Resources[0].URI)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := service.Resolve(context.Background(), nested.ID)
	if err != nil || resolved.Format != "text" {
		t.Fatalf("nested artifact was not cached: %#v err=%v", resolved, err)
	}
}

func TestServiceRejectsPathsOutsideRootsAndSymlinkEscapes(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(artifactkit.New(), Options{Roots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Inspect(context.Background(), outside); err == nil {
		t.Fatal("outside path was accepted")
	}
	link := filepath.Join(root, "escape.txt")
	if err := os.Symlink(outside, link); err == nil {
		if _, err := service.Inspect(context.Background(), link); err == nil {
			t.Fatal("symlink escape was accepted")
		}
	}
}

func TestServiceListsRootScopedFilesWithoutFollowingSymlinks(t *testing.T) {
	root := t.TempDir()
	subdirectory := filepath.Join(root, "nested")
	if err := os.Mkdir(subdirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		filepath.Join(root, "a.txt"):          "a",
		filepath.Join(subdirectory, "b.json"): "{}",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Symlink(outside, filepath.Join(root, "escape.txt"))

	service, err := NewService(artifactkit.New(), Options{Roots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	flat, err := service.ListFiles(context.Background(), root, false, 10)
	if err != nil || len(flat.Files) != 1 || flat.Files[0].RelativePath != "a.txt" {
		t.Fatalf("unexpected flat list: %#v err=%v", flat, err)
	}
	recursive, err := service.ListFiles(context.Background(), root, true, 1)
	if err != nil || len(recursive.Files) != 1 || !recursive.Truncated {
		t.Fatalf("unexpected recursive list: %#v err=%v", recursive, err)
	}
	if _, err := service.ListFiles(context.Background(), filepath.Dir(outside), true, 10); err == nil {
		t.Fatal("outside directory was accepted")
	}
}

func TestServiceEvictsOldDocuments(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(artifactkit.New(), Options{Roots: []string{root}, MaxDocuments: 1})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Inspect(context.Background(), filepath.Join(root, "incident.md"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Inspect(context.Background(), filepath.Join(root, "config.json")); err != nil {
		t.Fatal(err)
	}
	_, err = service.Resolve(context.Background(), first.ID)
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound, got %v", err)
	}
}

func TestSummarizeCapsOutline(t *testing.T) {
	document := &artifact.Artifact{Root: artifact.Node{Kind: artifact.KindDocument}}
	for i := 0; i < 250; i++ {
		document.Root.Children = append(document.Root.Children, artifact.Node{Kind: artifact.KindSection})
	}
	summary := Summarize(document)
	if len(summary.Outline) != 200 || summary.Nodes != 251 {
		t.Fatalf("unexpected summary bounds: %#v", summary)
	}
}
