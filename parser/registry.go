package parser

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/salman0ansari/artifactkit/artifact"
)

var ErrUnsupported = errors.New("artifactkit: unsupported artifact format")

// Detection is a candidate parser selected for an input.
type Detection struct {
	Parser Parser
	Match  Match
}

// Registry is safe for concurrent detection and parsing.
type Registry struct {
	mu      sync.RWMutex
	parsers []Parser
	names   map[string]struct{}
}

func NewRegistry() *Registry { return &Registry{names: make(map[string]struct{})} }

// Register adds a parser. Parser names must be unique and registration order breaks score ties.
func (r *Registry) Register(p Parser) error {
	if p == nil || p.Name() == "" {
		return fmt.Errorf("artifactkit: parser and parser name are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.names[p.Name()]; exists {
		return fmt.Errorf("artifactkit: parser %q already registered", p.Name())
	}
	r.names[p.Name()] = struct{}{}
	r.parsers = append(r.parsers, p)
	return nil
}

// Detect returns matching parsers by confidence, then registration order.
func (r *Registry) Detect(source Source) []Detection {
	r.mu.RLock()
	parsers := append([]Parser(nil), r.parsers...)
	r.mu.RUnlock()

	detections := make([]Detection, 0, len(parsers))
	for _, p := range parsers {
		match := p.Probe(source)
		if match.Score <= 0 {
			continue
		}
		if match.Score > 100 {
			match.Score = 100
		}
		detections = append(detections, Detection{Parser: p, Match: match})
	}
	sort.SliceStable(detections, func(i, j int) bool {
		return detections[i].Match.Score > detections[j].Match.Score
	})
	return detections
}

// Formats returns a de-duplicated, stable catalog of registered formats.
func (r *Registry) Formats() []artifact.Format {
	r.mu.RLock()
	parsers := append([]Parser(nil), r.parsers...)
	r.mu.RUnlock()

	seen := make(map[string]struct{})
	var formats []artifact.Format
	for _, p := range parsers {
		for _, f := range p.Formats() {
			if _, exists := seen[f.Name]; exists {
				continue
			}
			seen[f.Name] = struct{}{}
			formats = append(formats, f)
		}
	}
	sort.Slice(formats, func(i, j int) bool { return formats[i].Name < formats[j].Name })
	return formats
}
