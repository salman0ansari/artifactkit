// Package artifactkit turns local files into typed, provenance-preserving artifact trees.
package artifactkit

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
	"github.com/salman0ansari/artifactkit/parsers/archive"
	"github.com/salman0ansari/artifactkit/parsers/config"
	"github.com/salman0ansari/artifactkit/parsers/delimited"
	"github.com/salman0ansari/artifactkit/parsers/email"
	imageinfo "github.com/salman0ansari/artifactkit/parsers/image"
	"github.com/salman0ansari/artifactkit/parsers/jsondoc"
	"github.com/salman0ansari/artifactkit/parsers/ooxml"
	pdfparser "github.com/salman0ansari/artifactkit/parsers/pdf"
	"github.com/salman0ansari/artifactkit/parsers/text"
	"github.com/salman0ansari/artifactkit/parsers/web"
	"github.com/salman0ansari/artifactkit/resource"
)

// Engine detects and parses artifacts using a bounded parser registry.
type Engine struct {
	registry                *parser.Registry
	limits                  artifact.Limits
	resources               *resource.Store
	resourceStoreConfigured bool
}

// New returns an engine with the built-in parsers and conservative safety limits.
func New(options ...Option) *Engine {
	engine := &Engine{registry: defaultRegistry(), limits: artifact.DefaultLimits()}
	for _, option := range options {
		if option != nil {
			option(engine)
		}
	}
	if !engine.resourceStoreConfigured {
		engine.resources = resource.NewStore(engine.limits)
	}
	return engine
}

func defaultRegistry() *parser.Registry {
	registry := parser.NewRegistry()
	for _, candidate := range []parser.Parser{
		config.New(), jsondoc.New(), delimited.New(), web.NewHTML(), web.NewXML(), email.New(), imageinfo.New(), ooxml.New(), pdfparser.New(), archive.New(), text.New(),
	} {
		if err := registry.Register(candidate); err != nil {
			panic(err)
		}
	}
	return registry
}

// Formats lists formats supported by the current engine registry.
func (e *Engine) Formats() []artifact.Format { return e.registry.Formats() }

// Limits returns the engine's immutable parser and resource bounds.
func (e *Engine) Limits() artifact.Limits { return e.limits }

// InspectPath parses a regular file from disk.
func (e *Engine) InspectPath(ctx context.Context, path string) (*artifact.Artifact, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("inspect %q: not a regular file", path)
	}
	if info.Size() > e.limits.MaxInputBytes {
		return nil, &artifact.LimitError{Limit: "input bytes", Value: info.Size(), Max: e.limits.MaxInputBytes}
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("inspect %q: %w", path, err)
	}
	defer file.Close()
	return e.inspectReader(ctx, filepath.Base(path), path, file)
}

// InspectReader parses a named stream. Name should include an extension when known.
func (e *Engine) InspectReader(ctx context.Context, name string, reader io.Reader) (*artifact.Artifact, error) {
	return e.inspectReader(ctx, name, "", reader)
}

// InspectBytes parses an in-memory artifact.
func (e *Engine) InspectBytes(ctx context.Context, name string, data []byte) (*artifact.Artifact, error) {
	if int64(len(data)) > e.limits.MaxInputBytes {
		return nil, &artifact.LimitError{Limit: "input bytes", Value: int64(len(data)), Max: e.limits.MaxInputBytes}
	}
	return e.inspect(ctx, parser.NewSource(name, "", data))
}

func (e *Engine) inspectReader(ctx context.Context, name, path string, reader io.Reader) (*artifact.Artifact, error) {
	if err := e.limits.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(reader, e.limits.MaxInputBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", name, err)
	}
	if int64(len(data)) > e.limits.MaxInputBytes {
		return nil, &artifact.LimitError{Limit: "input bytes", Value: int64(len(data)), Max: e.limits.MaxInputBytes}
	}
	return e.inspect(ctx, parser.NewSource(name, path, data))
}

func (e *Engine) inspect(ctx context.Context, source parser.Source) (*artifact.Artifact, error) {
	if err := e.limits.Validate(); err != nil {
		return nil, err
	}
	detections := e.registry.Detect(source)
	if len(detections) == 0 {
		return nil, fmt.Errorf("%w: %s", parser.ErrUnsupported, source.Name)
	}
	selected := detections[0]
	result, err := selected.Parser.Parse(ctx, source, e.limits)
	if err != nil {
		return nil, fmt.Errorf("parse %q with %s: %w", source.Name, selected.Parser.Name(), err)
	}
	if result.Name == "" {
		result.Name = source.Name
	}
	if err := artifact.Finalize(result, source.Data, e.limits); err != nil {
		return nil, fmt.Errorf("finalize %q: %w", source.Name, err)
	}
	if e.resources != nil {
		e.resources.Put(result, source.Data)
	}
	return result, nil
}

// ReadResource reads a bounded byte range from an attachment or embedded package part.
func (e *Engine) ReadResource(ctx context.Context, uri string, offset, limit int64) (*resource.Content, error) {
	if e.resources == nil {
		return nil, resource.ErrExpired
	}
	return e.resources.Read(ctx, uri, offset, limit)
}

// InspectResource parses a lazy attachment or container entry as a new artifact.
// The resource must have been registered by an earlier inspection on this engine.
func (e *Engine) InspectResource(ctx context.Context, uri string) (*artifact.Artifact, error) {
	if e.resources == nil {
		return nil, resource.ErrExpired
	}
	content, err := e.resources.ReadAll(ctx, uri, e.limits.MaxInputBytes)
	if err != nil {
		return nil, fmt.Errorf("inspect resource %q: %w", uri, err)
	}
	document, err := e.InspectBytes(ctx, content.Name, content.Data)
	if err != nil {
		return nil, fmt.Errorf("inspect resource %q: %w", uri, err)
	}
	return document, nil
}

// ResourceStats reports the current bounded source-byte cache usage.
func (e *Engine) ResourceStats() resource.Stats {
	if e.resources == nil {
		return resource.Stats{}
	}
	return e.resources.Stats()
}

// ClearResources immediately releases all retained source bytes.
func (e *Engine) ClearResources() {
	if e.resources != nil {
		e.resources.Clear()
	}
}
