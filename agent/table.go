package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/salman0ansari/artifactkit/artifact"
)

type TableCell struct {
	NodeID     string            `json:"node_id"`
	Cell       string            `json:"cell,omitempty"`
	Value      string            `json:"value"`
	Locator    artifact.Locator  `json:"locator,omitzero"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type TableResult struct {
	ArtifactID string           `json:"artifact_id"`
	NodeID     string           `json:"node_id"`
	Name       string           `json:"name,omitempty"`
	Locator    artifact.Locator `json:"locator,omitzero"`
	Columns    []TableCell      `json:"columns,omitempty"`
	Rows       [][]TableCell    `json:"rows"`
	Offset     int              `json:"offset"`
	TotalRows  int              `json:"total_rows"`
	More       bool             `json:"more"`
}

func (s *Service) ExtractTable(ctx context.Context, reference, selector string, offset, limit int) (*TableResult, error) {
	document, err := s.Resolve(ctx, reference)
	if err != nil {
		return nil, err
	}
	if offset < 0 || limit < 0 {
		return nil, fmt.Errorf("artifactkit: table range must be non-negative")
	}
	if limit == 0 {
		limit = 50
	}
	if limit > 500 {
		return nil, fmt.Errorf("artifactkit: table row limit cannot exceed 500")
	}
	table := selectTable(&document.Root, selector)
	if table == nil {
		return nil, fmt.Errorf("artifactkit: table %q not found", selector)
	}
	result := &TableResult{ArtifactID: document.ID, NodeID: table.ID, Name: table.Name, Locator: table.Locator, Offset: offset}
	if len(table.Children) == 0 {
		return result, nil
	}
	result.Columns = tableRow(table.Children[0])
	dataRows := table.Children[1:]
	result.TotalRows = len(dataRows)
	if offset > len(dataRows) {
		offset = len(dataRows)
		result.Offset = offset
	}
	end := offset + limit
	if end > len(dataRows) {
		end = len(dataRows)
	}
	for _, row := range dataRows[offset:end] {
		result.Rows = append(result.Rows, tableRow(row))
	}
	result.More = end < len(dataRows)
	return result, nil
}

func selectTable(root *artifact.Node, selector string) *artifact.Node {
	if selector != "" {
		if selected := artifact.FindNode(root, selector); selected != nil {
			if selected.Kind == artifact.KindTable {
				return selected
			}
			if selected.Kind == artifact.KindSheet {
				for index := range selected.Children {
					if selected.Children[index].Kind == artifact.KindTable {
						return &selected.Children[index]
					}
				}
			}
		}
	}
	var match *artifact.Node
	artifact.Walk(root, func(node *artifact.Node) bool {
		if match != nil {
			return false
		}
		if node.Kind != artifact.KindTable {
			return true
		}
		if selector == "" || strings.EqualFold(node.Name, selector) {
			match = node
			return false
		}
		return true
	})
	return match
}

func tableRow(row artifact.Node) []TableCell {
	cells := make([]TableCell, 0, len(row.Children))
	for _, cell := range row.Children {
		cells = append(cells, TableCell{NodeID: cell.ID, Cell: cell.Name, Value: cell.Text, Locator: cell.Locator, Attributes: cell.Attributes})
	}
	return cells
}
