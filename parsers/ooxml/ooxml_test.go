package ooxml

import (
	"context"
	"os"
	"testing"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
)

func TestDOCXExtractsHeadingsTablesLinksAndMedia(t *testing.T) {
	document := parseFixture(t, "brief.docx")
	if document.Format != "docx" || document.Metadata["title"] != "Agent Brief" {
		t.Fatalf("unexpected DOCX metadata: %#v", document.Metadata)
	}
	var heading, linkedParagraph, tableCell, header bool
	artifact.Walk(&document.Root, func(node *artifact.Node) bool {
		heading = heading || (node.Kind == artifact.KindSection && node.Name == "Agent Brief" && node.Level == 1)
		linkedParagraph = linkedParagraph || node.Attributes["links"] == "https://example.test/runbook"
		tableCell = tableCell || (node.Kind == artifact.KindCell && node.Name == "B2" && node.Text == "ready" && node.Locator.Path == "word/document.xml")
		header = header || (node.Kind == artifact.KindParagraph && node.Text == "Internal agent report" && node.Locator.Path == "word/header1.xml")
		return true
	})
	if !heading || !linkedParagraph || !tableCell || !header {
		t.Fatalf("missing DOCX semantics: heading=%v link=%v cell=%v header=%v", heading, linkedParagraph, tableCell, header)
	}
	if len(document.Resources) != 1 || document.Resources[0].Locator.Path != "word/media/pixel.png" {
		t.Fatalf("unexpected DOCX resources: %#v", document.Resources)
	}
}

func TestPPTXExtractsSlidesTablesAndNotes(t *testing.T) {
	document := parseFixture(t, "deck.pptx")
	if document.Format != "pptx" || len(document.Root.Children) != 1 {
		t.Fatalf("unexpected PPTX root: %#v", document.Root)
	}
	slide := document.Root.Children[0]
	if slide.Kind != artifact.KindSlide || slide.Name != "Agent Deck" || slide.Locator.Slide != 1 {
		t.Fatalf("unexpected slide: %#v", slide)
	}
	var cell, notes bool
	artifact.Walk(&slide, func(node *artifact.Node) bool {
		cell = cell || (node.Kind == artifact.KindCell && node.Name == "B2" && node.Text == "ready" && node.Locator.Slide == 1)
		notes = notes || node.Text == "Speaker note: cite slide one."
		return true
	})
	if !cell || !notes {
		t.Fatalf("missing PPTX data: cell=%v notes=%v", cell, notes)
	}
}

func TestXLSXExtractsSharedInlineFormulaAndHyperlinkCells(t *testing.T) {
	document := parseFixture(t, "workbook.xlsx")
	if document.Format != "xlsx" || document.Metadata["date_system"] != "1900" {
		t.Fatalf("unexpected XLSX metadata: %#v", document.Metadata)
	}
	var agent, boolean, formula bool
	artifact.Walk(&document.Root, func(node *artifact.Node) bool {
		switch node.Name {
		case "A1":
			agent = node.Text == "Agent" && node.Locator.Sheet == "Agents" && node.Attributes["hyperlink"] == "https://example.test/agents"
		case "B2":
			boolean = node.Text == "true"
		case "C2":
			formula = node.Text == "3" && node.Attributes["formula"] == "SUM(1,2)"
		}
		return true
	})
	if !agent || !boolean || !formula {
		t.Fatalf("missing XLSX cells: agent=%v boolean=%v formula=%v", agent, boolean, formula)
	}
	table := document.Root.Children[0].Children[0]
	if table.Attributes["merged_ranges"] != "C3:D3" {
		t.Fatalf("merged ranges = %q", table.Attributes["merged_ranges"])
	}
}

func parseFixture(t *testing.T, name string) *artifact.Artifact {
	t.Helper()
	data, err := os.ReadFile("../../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	document, err := New().Parse(context.Background(), parser.NewSource(name, "", data), artifact.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	return document
}
