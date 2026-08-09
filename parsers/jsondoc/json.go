// Package jsondoc parses JSON into a typed tree with JSON Pointer provenance.
package jsondoc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
)

type Parser struct{}

func New() *Parser           { return &Parser{} }
func (*Parser) Name() string { return "json" }

func (*Parser) Formats() []artifact.Format {
	return []artifact.Format{{Name: "json", MediaType: "application/json", Extensions: []string{".json"}}}
}

func (*Parser) Probe(source parser.Source) parser.Match {
	trimmed := bytes.TrimSpace(source.Data)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid(trimmed) {
		return parser.Match{Score: 100, Reason: "valid JSON object or array"}
	}
	if source.Extension() == ".json" {
		return parser.Match{Score: 70, Reason: "JSON extension"}
	}
	return parser.Match{}
}

func (*Parser) Parse(ctx context.Context, source parser.Source, limits artifact.Limits) (*artifact.Artifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(source.Data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return nil, fmt.Errorf("parse JSON: multiple top-level values")
	} else if err != io.EOF {
		return nil, fmt.Errorf("parse JSON: trailing data: %w", err)
	}

	builder := treeBuilder{limits: limits}
	child, err := builder.node(ctx, value, "", "root", 1)
	if err != nil {
		return nil, err
	}
	return &artifact.Artifact{
		Name: source.Name, Format: "json", MediaType: "application/json",
		Root: artifact.Node{Kind: artifact.KindDocument, Name: source.Name, Children: []artifact.Node{child}},
	}, nil
}

type treeBuilder struct {
	limits artifact.Limits
	nodes  int
}

func (b *treeBuilder) node(ctx context.Context, value any, pointer, name string, depth int) (artifact.Node, error) {
	if err := ctx.Err(); err != nil {
		return artifact.Node{}, err
	}
	if depth > b.limits.MaxNestingDepth {
		return artifact.Node{}, &artifact.LimitError{Limit: "nesting depth", Value: int64(depth), Max: int64(b.limits.MaxNestingDepth)}
	}
	b.nodes++
	if b.nodes > b.limits.MaxNodes {
		return artifact.Node{}, &artifact.LimitError{Limit: "nodes", Value: int64(b.nodes), Max: int64(b.limits.MaxNodes)}
	}
	locator := artifact.Locator{Path: pointer}
	switch typed := value.(type) {
	case map[string]any:
		n := artifact.Node{Kind: artifact.KindObject, Name: name, Locator: locator}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			child, err := b.node(ctx, typed[key], pointer+"/"+escapePointer(key), key, depth+1)
			if err != nil {
				return artifact.Node{}, err
			}
			field := artifact.Node{Kind: artifact.KindField, Name: key, Locator: child.Locator, Children: []artifact.Node{child}}
			n.Children = append(n.Children, field)
		}
		return n, nil
	case []any:
		n := artifact.Node{Kind: artifact.KindArray, Name: name, Locator: locator}
		for index, item := range typed {
			child, err := b.node(ctx, item, pointer+"/"+strconv.Itoa(index), strconv.Itoa(index), depth+1)
			if err != nil {
				return artifact.Node{}, err
			}
			n.Children = append(n.Children, child)
		}
		return n, nil
	case nil:
		return artifact.Node{Kind: artifact.KindValue, Name: name, Text: "null", Locator: locator, Attributes: map[string]string{"type": "null"}}, nil
	case bool:
		return artifact.Node{Kind: artifact.KindValue, Name: name, Text: strconv.FormatBool(typed), Locator: locator, Attributes: map[string]string{"type": "boolean"}}, nil
	case json.Number:
		return artifact.Node{Kind: artifact.KindValue, Name: name, Text: typed.String(), Locator: locator, Attributes: map[string]string{"type": "number"}}, nil
	case string:
		return artifact.Node{Kind: artifact.KindValue, Name: name, Text: typed, Locator: locator, Attributes: map[string]string{"type": "string"}}, nil
	default:
		return artifact.Node{}, fmt.Errorf("parse JSON: unsupported value type %T", value)
	}
}

func escapePointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}
