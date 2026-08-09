// Package artifact defines ArtifactKit's stable, format-independent document model.
package artifact

import (
	"fmt"
	"strings"
)

// Kind describes the semantic role of a node in an artifact tree.
type Kind string

const (
	KindDocument   Kind = "document"
	KindSection    Kind = "section"
	KindParagraph  Kind = "paragraph"
	KindTable      Kind = "table"
	KindRow        Kind = "row"
	KindCell       Kind = "cell"
	KindSlide      Kind = "slide"
	KindPage       Kind = "page"
	KindSheet      Kind = "sheet"
	KindImage      Kind = "image"
	KindLink       Kind = "link"
	KindAttachment Kind = "attachment"
	KindArchive    Kind = "archive"
	KindEntry      Kind = "entry"
	KindObject     Kind = "object"
	KindArray      Kind = "array"
	KindField      Kind = "field"
	KindValue      Kind = "value"
	KindMetadata   Kind = "metadata"
	KindElement    Kind = "element"
)

// ByteRange is a half-open byte interval in the original source.
type ByteRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// Locator records where content came from. Fields are intentionally additive:
// a spreadsheet cell inside an archive can have path, sheet, cell, and bytes.
type Locator struct {
	Path      string     `json:"path,omitempty"`
	Subpath   string     `json:"subpath,omitempty"`
	Page      int        `json:"page,omitempty"`
	Slide     int        `json:"slide,omitempty"`
	Sheet     string     `json:"sheet,omitempty"`
	Cell      string     `json:"cell,omitempty"`
	ByteRange *ByteRange `json:"byte_range,omitempty"`
}

// IsZero lets JSON encoders omit absent provenance cleanly.
func (l Locator) IsZero() bool {
	return l.Path == "" && l.Subpath == "" && l.Page == 0 && l.Slide == 0 && l.Sheet == "" && l.Cell == "" && l.ByteRange == nil
}

func (l Locator) String() string {
	parts := make([]string, 0, 6)
	if l.Path != "" {
		parts = append(parts, l.Path)
	}
	if l.Subpath != "" {
		parts = append(parts, l.Subpath)
	}
	if l.Page > 0 {
		parts = append(parts, fmt.Sprintf("page:%d", l.Page))
	}
	if l.Slide > 0 {
		parts = append(parts, fmt.Sprintf("slide:%d", l.Slide))
	}
	if l.Sheet != "" {
		parts = append(parts, "sheet:"+l.Sheet)
	}
	if l.Cell != "" {
		parts = append(parts, "cell:"+l.Cell)
	}
	if l.ByteRange != nil {
		parts = append(parts, fmt.Sprintf("bytes:%d-%d", l.ByteRange.Start, l.ByteRange.End))
	}
	return strings.Join(parts, "#")
}

// Node is one semantic unit in an artifact. IDs are deterministic for identical input.
type Node struct {
	ID         string            `json:"id"`
	Kind       Kind              `json:"kind"`
	Name       string            `json:"name,omitempty"`
	Text       string            `json:"text,omitempty"`
	Level      int               `json:"level,omitempty"`
	Locator    Locator           `json:"locator,omitzero"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Children   []Node            `json:"children,omitempty"`
}

// Resource describes content that may be read lazily by its stable URI.
type Resource struct {
	URI       string  `json:"uri"`
	Name      string  `json:"name"`
	MediaType string  `json:"media_type,omitempty"`
	Size      int64   `json:"size,omitempty"`
	Locator   Locator `json:"locator,omitzero"`
}

// Warning reports a recoverable parse condition.
type Warning struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Locator Locator `json:"locator,omitzero"`
}

// Artifact is the normalized representation returned by every parser.
type Artifact struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Format    string            `json:"format"`
	MediaType string            `json:"media_type"`
	Size      int64             `json:"size"`
	Digest    string            `json:"digest"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Root      Node              `json:"root"`
	Resources []Resource        `json:"resources,omitempty"`
	Warnings  []Warning         `json:"warnings,omitempty"`
}

// Format advertises one format understood by a parser.
type Format struct {
	Name       string   `json:"name"`
	MediaType  string   `json:"media_type"`
	Extensions []string `json:"extensions"`
}

// Walk visits root and every descendant in document order. Returning false skips a subtree.
func Walk(root *Node, visit func(*Node) bool) {
	if root == nil || !visit(root) {
		return
	}
	for i := range root.Children {
		Walk(&root.Children[i], visit)
	}
}

// FindNode returns a node by deterministic ID.
func FindNode(root *Node, id string) *Node {
	var found *Node
	Walk(root, func(node *Node) bool {
		if node.ID == id {
			found = node
			return false
		}
		return found == nil
	})
	return found
}
