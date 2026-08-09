// Package parser contains the parser contract and deterministic format registry.
package parser

import (
	"context"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/salman0ansari/artifactkit/artifact"
)

// Source is an immutable parser input. Data is bounded by Engine limits before parsing.
type Source struct {
	Name      string
	Path      string
	MediaType string
	Data      []byte
}

// NewSource creates a source and fills its media type from extension or content sniffing.
func NewSource(name, path string, data []byte) Source {
	mediaType := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	if mediaType == "" {
		mediaType = http.DetectContentType(data)
	}
	if semi := strings.IndexByte(mediaType, ';'); semi >= 0 {
		mediaType = mediaType[:semi]
	}
	return Source{Name: name, Path: path, MediaType: mediaType, Data: data}
}

func (s Source) Extension() string { return strings.ToLower(filepath.Ext(s.Name)) }
func (s Source) Size() int64       { return int64(len(s.Data)) }

// Match is a parser's confidence score and explanation. Scores range from 0 to 100.
type Match struct {
	Score  int
	Reason string
}

// Parser converts one or more source formats into an Artifact.
type Parser interface {
	Name() string
	Formats() []artifact.Format
	Probe(Source) Match
	Parse(context.Context, Source, artifact.Limits) (*artifact.Artifact, error)
}
