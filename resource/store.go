// Package resource stores source bytes temporarily and serves bounded lazy reads.
package resource

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/salman0ansari/artifactkit/artifact"
)

var (
	ErrNotFound = errors.New("artifactkit: resource not found")
	ErrExpired  = errors.New("artifactkit: source bytes are not in the resource store")
)

type Content struct {
	URI       string `json:"uri"`
	Name      string `json:"name"`
	MediaType string `json:"media_type,omitempty"`
	Size      int64  `json:"size"`
	Offset    int64  `json:"offset"`
	Data      []byte `json:"data"`
	EOF       bool   `json:"eof"`
}

type Stats struct {
	Artifacts int   `json:"artifacts"`
	Bytes     int64 `json:"bytes"`
	MaxBytes  int64 `json:"max_bytes"`
}

type storedArtifact struct {
	digest    string
	format    string
	data      []byte
	resources map[string]artifact.Resource
	element   *list.Element
}

// Store is a concurrency-safe LRU cache. Eviction removes source bytes, never caller artifacts.
type Store struct {
	mu           sync.Mutex
	limits       artifact.Limits
	items        map[string]*storedArtifact
	recency      *list.List
	currentBytes int64
}

func NewStore(limits artifact.Limits) *Store {
	return &Store{limits: limits, items: make(map[string]*storedArtifact), recency: list.New()}
}

// Put retains source bytes only when the artifact exposes lazy resources.
func (s *Store) Put(document *artifact.Artifact, source []byte) bool {
	if s == nil || document == nil || len(document.Resources) == 0 || int64(len(source)) > s.limits.MaxStoredBytes {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.items[document.Digest]; existing != nil {
		for _, item := range document.Resources {
			existing.resources[item.URI] = item
		}
		existing.format = document.Format
		s.recency.MoveToFront(existing.element)
		return true
	}
	for s.currentBytes+int64(len(source)) > s.limits.MaxStoredBytes || len(s.items) >= s.limits.MaxStoredArtifacts {
		if !s.evictOldest() {
			return false
		}
	}
	resources := make(map[string]artifact.Resource, len(document.Resources))
	for _, item := range document.Resources {
		resources[item.URI] = item
	}
	record := &storedArtifact{
		digest: document.Digest, format: document.Format, data: append([]byte(nil), source...), resources: resources,
	}
	record.element = s.recency.PushFront(record)
	s.items[record.digest] = record
	s.currentBytes += int64(len(record.data))
	return true
}

func (s *Store) Read(ctx context.Context, uri string, offset, limit int64) (*Content, error) {
	if s == nil {
		return nil, ErrExpired
	}
	if offset < 0 {
		return nil, fmt.Errorf("artifactkit: resource offset must be non-negative")
	}
	if limit == 0 {
		limit = s.limits.MaxResourceReadBytes
	}
	if limit < 0 || limit > s.limits.MaxResourceReadBytes {
		return nil, fmt.Errorf("artifactkit: resource read limit must be between 1 and %d", s.limits.MaxResourceReadBytes)
	}
	digest, err := digestFromURI(uri)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	record := s.items[digest]
	if record == nil {
		s.mu.Unlock()
		return nil, ErrExpired
	}
	item, exists := record.resources[uri]
	if !exists {
		s.mu.Unlock()
		return nil, ErrNotFound
	}
	s.recency.MoveToFront(record.element)
	format := record.format
	data := record.data
	s.mu.Unlock()

	content, err := readStored(ctx, format, data, item, offset, limit, s.limits)
	if err != nil {
		return nil, err
	}
	content.URI = uri
	content.Name = item.Name
	content.MediaType = item.MediaType
	content.Size = item.Size
	content.Offset = offset
	return content, nil
}

// ReadAll extracts an entire lazy resource for bounded recursive inspection.
// maxBytes is independent from the smaller range-read limit used by Read.
func (s *Store) ReadAll(ctx context.Context, uri string, maxBytes int64) (*Content, error) {
	if s == nil {
		return nil, ErrExpired
	}
	if maxBytes <= 0 {
		return nil, fmt.Errorf("artifactkit: resource inspection limit must be positive")
	}
	digest, err := digestFromURI(uri)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	record := s.items[digest]
	if record == nil {
		s.mu.Unlock()
		return nil, ErrExpired
	}
	item, exists := record.resources[uri]
	if !exists {
		s.mu.Unlock()
		return nil, ErrNotFound
	}
	if item.Size < 0 {
		s.mu.Unlock()
		return nil, fmt.Errorf("artifactkit: resource has a negative declared size")
	}
	if item.Size > maxBytes {
		s.mu.Unlock()
		return nil, &artifact.LimitError{Limit: "input bytes", Value: item.Size, Max: maxBytes}
	}
	s.recency.MoveToFront(record.element)
	format := record.format
	data := record.data
	s.mu.Unlock()

	content, err := readStored(ctx, format, data, item, 0, item.Size, s.limits)
	if err != nil {
		return nil, err
	}
	content.URI = uri
	content.Name = item.Name
	content.MediaType = item.MediaType
	content.Size = item.Size
	return content, nil
}

func (s *Store) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = make(map[string]*storedArtifact)
	s.recency.Init()
	s.currentBytes = 0
}

func (s *Store) Stats() Stats {
	if s == nil {
		return Stats{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return Stats{Artifacts: len(s.items), Bytes: s.currentBytes, MaxBytes: s.limits.MaxStoredBytes}
}

func (s *Store) evictOldest() bool {
	element := s.recency.Back()
	if element == nil {
		return false
	}
	record := element.Value.(*storedArtifact)
	delete(s.items, record.digest)
	s.currentBytes -= int64(len(record.data))
	s.recency.Remove(element)
	return true
}

func digestFromURI(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "artifact" || len(parsed.Host) != 64 || strings.Trim(parsed.Host, "0123456789abcdef") != "" {
		return "", fmt.Errorf("artifactkit: invalid resource URI %q", value)
	}
	return parsed.Host, nil
}
