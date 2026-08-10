// Package agent exposes bounded artifact operations shaped for tool-calling agents.
package agent

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	artifactkit "github.com/salman0ansari/artifactkit"
	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/search"
)

var ErrArtifactNotFound = errors.New("artifactkit: artifact is not loaded")

type Options struct {
	Roots        []string
	MaxDocuments int
}

type documentRecord struct {
	document *artifact.Artifact
	paths    map[string]struct{}
	element  *list.Element
}

type Service struct {
	engine       *artifactkit.Engine
	roots        []string
	maxDocuments int
	mu           sync.Mutex
	documents    map[string]*documentRecord
	paths        map[string]string
	recency      *list.List
}

func NewService(engine *artifactkit.Engine, options Options) (*Service, error) {
	if engine == nil {
		engine = artifactkit.New()
	}
	roots := options.Roots
	if len(roots) == 0 {
		workingDirectory, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		roots = []string{workingDirectory}
	}
	resolvedRoots := make([]string, 0, len(roots))
	for _, root := range roots {
		absolute, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("resolve root %q: %w", root, err)
		}
		real, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return nil, fmt.Errorf("resolve root %q: %w", root, err)
		}
		info, err := os.Stat(real)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("artifactkit: root %q is not a directory", root)
		}
		resolvedRoots = append(resolvedRoots, filepath.Clean(real))
	}
	maxDocuments := options.MaxDocuments
	if maxDocuments <= 0 {
		maxDocuments = engine.Limits().MaxStoredArtifacts
	}
	return &Service{
		engine: engine, roots: resolvedRoots, maxDocuments: maxDocuments,
		documents: make(map[string]*documentRecord), paths: make(map[string]string), recency: list.New(),
	}, nil
}

func (s *Service) Inspect(ctx context.Context, filePath string) (*artifact.Artifact, error) {
	securePath, err := s.securePath(filePath)
	if err != nil {
		return nil, err
	}
	document, err := s.engine.InspectPath(ctx, securePath)
	if err != nil {
		return nil, err
	}
	return s.remember(document, securePath), nil
}

// InspectResource parses a resource exposed by an already-loaded artifact.
func (s *Service) InspectResource(ctx context.Context, uri string) (*artifact.Artifact, error) {
	document, err := s.engine.InspectResource(ctx, uri)
	if err != nil {
		return nil, err
	}
	return s.remember(document, ""), nil
}

func (s *Service) remember(document *artifact.Artifact, securePath string) *artifact.Artifact {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record := s.documents[document.ID]; record != nil {
		if securePath != "" {
			record.paths[securePath] = struct{}{}
			s.paths[securePath] = document.ID
		}
		s.recency.MoveToFront(record.element)
		return record.document
	}
	for len(s.documents) >= s.maxDocuments {
		s.evictOldest()
	}
	record := &documentRecord{document: document, paths: make(map[string]struct{})}
	if securePath != "" {
		record.paths[securePath] = struct{}{}
		s.paths[securePath] = document.ID
	}
	record.element = s.recency.PushFront(record)
	s.documents[document.ID] = record
	return document
}

func (s *Service) Resolve(ctx context.Context, reference string) (*artifact.Artifact, error) {
	s.mu.Lock()
	if record := s.documents[reference]; record != nil {
		s.recency.MoveToFront(record.element)
		document := record.document
		s.mu.Unlock()
		return document, nil
	}
	s.mu.Unlock()
	if strings.HasPrefix(reference, "sha256:") {
		return nil, ErrArtifactNotFound
	}
	return s.Inspect(ctx, reference)
}

func (s *Service) Find(ctx context.Context, reference, query string, limit int) ([]search.Result, error) {
	document, err := s.Resolve(ctx, reference)
	if err != nil {
		return nil, err
	}
	return search.Find(document, query, limit), nil
}

func (s *Service) ReadNode(ctx context.Context, reference, nodeID string, offset, limit int) (*NodeView, error) {
	document, err := s.Resolve(ctx, reference)
	if err != nil {
		return nil, err
	}
	node := artifact.FindNode(&document.Root, nodeID)
	if node == nil {
		return nil, fmt.Errorf("artifactkit: node %q not found", nodeID)
	}
	if offset < 0 || limit < 0 {
		return nil, fmt.Errorf("artifactkit: node range must be non-negative")
	}
	if limit == 0 {
		limit = 4_000
	}
	if limit > 50_000 {
		return nil, fmt.Errorf("artifactkit: node read limit cannot exceed 50000 characters")
	}
	runes := []rune(node.Text)
	if offset > len(runes) {
		offset = len(runes)
	}
	end := offset + limit
	if end > len(runes) {
		end = len(runes)
	}
	children := make([]string, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, child.ID)
	}
	return &NodeView{
		ArtifactID: document.ID, NodeID: node.ID, Kind: node.Kind, Name: node.Name,
		Text: string(runes[offset:end]), Offset: offset, More: end < len(runes), Locator: node.Locator,
		Attributes: node.Attributes, Children: children,
	}, nil
}

func (s *Service) Provenance(ctx context.Context, reference, nodeID string) (*Provenance, error) {
	document, err := s.Resolve(ctx, reference)
	if err != nil {
		return nil, err
	}
	var trail []Breadcrumb
	if !findTrail(&document.Root, nodeID, &trail) {
		return nil, fmt.Errorf("artifactkit: node %q not found", nodeID)
	}
	node := artifact.FindNode(&document.Root, nodeID)
	return &Provenance{ArtifactID: document.ID, NodeID: nodeID, Locator: node.Locator, Trail: trail}, nil
}

func (s *Service) Resources(ctx context.Context, reference string) ([]artifact.Resource, error) {
	document, err := s.Resolve(ctx, reference)
	if err != nil {
		return nil, err
	}
	return append([]artifact.Resource(nil), document.Resources...), nil
}

func (s *Service) Engine() *artifactkit.Engine { return s.engine }

func (s *Service) securePath(value string) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	for _, root := range s.roots {
		relative, err := filepath.Rel(root, real)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return real, nil
		}
	}
	return "", fmt.Errorf("artifactkit: path %q is outside configured roots", value)
}

func (s *Service) evictOldest() {
	element := s.recency.Back()
	if element == nil {
		return
	}
	record := element.Value.(*documentRecord)
	delete(s.documents, record.document.ID)
	for path := range record.paths {
		delete(s.paths, path)
	}
	s.recency.Remove(element)
}

func findTrail(node *artifact.Node, target string, trail *[]Breadcrumb) bool {
	*trail = append(*trail, Breadcrumb{NodeID: node.ID, Kind: node.Kind, Name: node.Name})
	if node.ID == target {
		return true
	}
	for index := range node.Children {
		if findTrail(&node.Children[index], target, trail) {
			return true
		}
	}
	*trail = (*trail)[:len(*trail)-1]
	return false
}
