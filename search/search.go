// Package search provides deterministic, dependency-free search over artifact trees.
package search

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/salman0ansari/artifactkit/artifact"
)

// Result points to a matching node without copying its subtree.
type Result struct {
	NodeID  string           `json:"node_id"`
	Kind    artifact.Kind    `json:"kind"`
	Name    string           `json:"name,omitempty"`
	Snippet string           `json:"snippet"`
	Score   int              `json:"score"`
	Locator artifact.Locator `json:"locator,omitempty"`
}

// Find performs case-insensitive token search and returns strongest matches first.
func Find(document *artifact.Artifact, query string, limit int) []Result {
	if document == nil || strings.TrimSpace(query) == "" || limit == 0 {
		return nil
	}
	if limit < 0 {
		limit = 20
	}
	tokens := strings.Fields(strings.ToLower(query))
	var results []Result
	artifact.Walk(&document.Root, func(node *artifact.Node) bool {
		haystack := strings.ToLower(strings.TrimSpace(node.Name + " " + node.Text))
		if haystack == "" {
			return true
		}
		score := 0
		for _, token := range tokens {
			count := strings.Count(haystack, token)
			if count == 0 {
				return true
			}
			score += 10 + count
			if strings.EqualFold(strings.TrimSpace(node.Name), token) {
				score += 20
			}
		}
		results = append(results, Result{
			NodeID: node.ID, Kind: node.Kind, Name: node.Name,
			Snippet: snippet(node), Score: score, Locator: node.Locator,
		})
		return true
	})
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].NodeID < results[j].NodeID
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

func snippet(node *artifact.Node) string {
	value := strings.TrimSpace(node.Text)
	if value == "" {
		value = node.Name
	}
	value = strings.Join(strings.Fields(value), " ")
	if utf8.RuneCountInString(value) <= 220 {
		return value
	}
	runes := []rune(value)
	return string(runes[:217]) + "..."
}
