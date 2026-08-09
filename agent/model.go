package agent

import (
	"github.com/salman0ansari/artifactkit/artifact"
)

type NodeView struct {
	ArtifactID string            `json:"artifact_id"`
	NodeID     string            `json:"node_id"`
	Kind       artifact.Kind     `json:"kind"`
	Name       string            `json:"name,omitempty"`
	Text       string            `json:"text,omitempty"`
	Offset     int               `json:"offset"`
	More       bool              `json:"more"`
	Locator    artifact.Locator  `json:"locator,omitzero"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Children   []string          `json:"children,omitempty"`
}

type Breadcrumb struct {
	NodeID string        `json:"node_id"`
	Kind   artifact.Kind `json:"kind"`
	Name   string        `json:"name,omitempty"`
}

type Provenance struct {
	ArtifactID string           `json:"artifact_id"`
	NodeID     string           `json:"node_id"`
	Locator    artifact.Locator `json:"locator,omitzero"`
	Trail      []Breadcrumb     `json:"trail"`
}

type Summary struct {
	ArtifactID string             `json:"artifact_id"`
	Name       string             `json:"name"`
	Format     string             `json:"format"`
	MediaType  string             `json:"media_type"`
	Size       int64              `json:"size"`
	Metadata   map[string]string  `json:"metadata,omitempty"`
	Nodes      int                `json:"nodes"`
	Resources  int                `json:"resources"`
	Warnings   []artifact.Warning `json:"warnings,omitempty"`
	Outline    []OutlineItem      `json:"outline,omitempty"`
}

type OutlineItem struct {
	NodeID  string           `json:"node_id"`
	Kind    artifact.Kind    `json:"kind"`
	Name    string           `json:"name,omitempty"`
	Locator artifact.Locator `json:"locator,omitzero"`
}

func Summarize(document *artifact.Artifact) Summary {
	summary := Summary{
		ArtifactID: document.ID, Name: document.Name, Format: document.Format, MediaType: document.MediaType,
		Size: document.Size, Metadata: document.Metadata, Resources: len(document.Resources), Warnings: document.Warnings,
	}
	artifact.Walk(&document.Root, func(node *artifact.Node) bool {
		summary.Nodes++
		if len(summary.Outline) < 200 {
			switch node.Kind {
			case artifact.KindSection, artifact.KindPage, artifact.KindSlide, artifact.KindSheet, artifact.KindTable, artifact.KindAttachment:
				summary.Outline = append(summary.Outline, OutlineItem{NodeID: node.ID, Kind: node.Kind, Name: node.Name, Locator: node.Locator})
			}
		}
		return true
	})
	return summary
}
