// Package config parses YAML and TOML into deterministic typed trees.
package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"go.yaml.in/yaml/v3"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
)

type Parser struct{}

func New() *Parser           { return &Parser{} }
func (*Parser) Name() string { return "config" }

func (*Parser) Formats() []artifact.Format {
	return []artifact.Format{
		{Name: "yaml", MediaType: "application/yaml", Extensions: []string{".yaml", ".yml"}},
		{Name: "toml", MediaType: "application/toml", Extensions: []string{".toml"}},
	}
}

func (*Parser) Probe(source parser.Source) parser.Match {
	switch source.Extension() {
	case ".yaml", ".yml":
		return parser.Match{Score: 100, Reason: "YAML extension"}
	case ".toml":
		return parser.Match{Score: 100, Reason: "TOML extension"}
	default:
		return parser.Match{}
	}
}

func (*Parser) Parse(ctx context.Context, source parser.Source, limits artifact.Limits) (*artifact.Artifact, error) {
	switch source.Extension() {
	case ".yaml", ".yml":
		return parseYAML(ctx, source, limits)
	case ".toml":
		return parseTOML(ctx, source, limits)
	default:
		return nil, fmt.Errorf("parse config: unsupported extension %q", source.Extension())
	}
}

func parseYAML(ctx context.Context, source parser.Source, limits artifact.Limits) (*artifact.Artifact, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(source.Data))
	var documents []*yaml.Node
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var document yaml.Node
		err := decoder.Decode(&document)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse YAML: %w", err)
		}
		if len(document.Content) > 0 {
			documents = append(documents, document.Content[0])
		}
	}

	builder := yamlBuilder{limits: limits}
	root := artifact.Node{Kind: artifact.KindDocument, Name: source.Name}
	metadata := map[string]string{"encoding": "utf-8", "documents": strconv.Itoa(len(documents))}
	if len(documents) == 0 {
		root.Children = []artifact.Node{{Kind: artifact.KindValue, Name: "root", Text: "null", Attributes: map[string]string{"type": "null"}}}
	} else if len(documents) == 1 {
		node, err := builder.node(ctx, documents[0], "", "root", 1, make(map[*yaml.Node]bool))
		if err != nil {
			return nil, err
		}
		root.Children = []artifact.Node{node}
	} else {
		array := artifact.Node{Kind: artifact.KindArray, Name: "documents"}
		for index, document := range documents {
			node, err := builder.node(ctx, document, "/"+strconv.Itoa(index), strconv.Itoa(index), 1, make(map[*yaml.Node]bool))
			if err != nil {
				return nil, err
			}
			array.Children = append(array.Children, node)
		}
		root.Children = []artifact.Node{array}
	}
	return &artifact.Artifact{Name: source.Name, Format: "yaml", MediaType: "application/yaml", Metadata: metadata, Root: root}, nil
}

type yamlBuilder struct {
	limits artifact.Limits
	nodes  int
}

func (b *yamlBuilder) node(ctx context.Context, value *yaml.Node, pointer, name string, depth int, aliases map[*yaml.Node]bool) (artifact.Node, error) {
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
	if value.Kind == yaml.AliasNode {
		if value.Alias == nil || aliases[value.Alias] {
			return artifact.Node{}, fmt.Errorf("parse YAML: cyclic or invalid alias")
		}
		aliases[value.Alias] = true
		node, err := b.node(ctx, value.Alias, pointer, name, depth+1, aliases)
		delete(aliases, value.Alias)
		return node, err
	}
	locator := artifact.Locator{Path: pointer}
	attributes := position(value)
	switch value.Kind {
	case yaml.MappingNode:
		if len(value.Content)%2 != 0 {
			return artifact.Node{}, fmt.Errorf("parse YAML: malformed mapping")
		}
		node := artifact.Node{Kind: artifact.KindObject, Name: name, Locator: locator, Attributes: attributes}
		seen := make(map[string]struct{}, len(value.Content)/2)
		for index := 0; index < len(value.Content); index += 2 {
			key := value.Content[index]
			if key.Kind != yaml.ScalarNode {
				return artifact.Node{}, fmt.Errorf("parse YAML: mapping keys must be scalars")
			}
			if _, exists := seen[key.Value]; exists {
				return artifact.Node{}, fmt.Errorf("parse YAML: duplicate key %q", key.Value)
			}
			seen[key.Value] = struct{}{}
			child, err := b.node(ctx, value.Content[index+1], pointer+"/"+escapePointer(key.Value), key.Value, depth+1, aliases)
			if err != nil {
				return artifact.Node{}, err
			}
			node.Children = append(node.Children, artifact.Node{Kind: artifact.KindField, Name: key.Value, Locator: child.Locator, Attributes: position(key), Children: []artifact.Node{child}})
		}
		return node, nil
	case yaml.SequenceNode:
		node := artifact.Node{Kind: artifact.KindArray, Name: name, Locator: locator, Attributes: attributes}
		for index, item := range value.Content {
			child, err := b.node(ctx, item, pointer+"/"+strconv.Itoa(index), strconv.Itoa(index), depth+1, aliases)
			if err != nil {
				return artifact.Node{}, err
			}
			node.Children = append(node.Children, child)
		}
		return node, nil
	case yaml.ScalarNode:
		attributes["type"] = yamlType(value.Tag)
		text := value.Value
		if attributes["type"] == "null" {
			text = "null"
		}
		return artifact.Node{Kind: artifact.KindValue, Name: name, Text: text, Locator: locator, Attributes: attributes}, nil
	default:
		return artifact.Node{}, fmt.Errorf("parse YAML: unsupported node kind %d", value.Kind)
	}
}

func parseTOML(ctx context.Context, source parser.Source, limits artifact.Limits) (*artifact.Artifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var decoded map[string]any
	if err := toml.Unmarshal(source.Data, &decoded); err != nil {
		return nil, fmt.Errorf("parse TOML: %w", err)
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return nil, fmt.Errorf("normalize TOML: %w", err)
	}
	jsonDecoder := json.NewDecoder(bytes.NewReader(encoded))
	jsonDecoder.UseNumber()
	var normalized any
	if err := jsonDecoder.Decode(&normalized); err != nil {
		return nil, fmt.Errorf("normalize TOML: %w", err)
	}
	builder := valueBuilder{limits: limits}
	node, err := builder.node(ctx, normalized, "", "root", 1)
	if err != nil {
		return nil, err
	}
	return &artifact.Artifact{
		Name: source.Name, Format: "toml", MediaType: "application/toml",
		Metadata: map[string]string{"encoding": "utf-8"},
		Root:     artifact.Node{Kind: artifact.KindDocument, Name: source.Name, Children: []artifact.Node{node}},
	}, nil
}

type valueBuilder struct {
	limits artifact.Limits
	nodes  int
}

func (b *valueBuilder) node(ctx context.Context, value any, pointer, name string, depth int) (artifact.Node, error) {
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
		node := artifact.Node{Kind: artifact.KindObject, Name: name, Locator: locator}
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
			node.Children = append(node.Children, artifact.Node{Kind: artifact.KindField, Name: key, Locator: child.Locator, Children: []artifact.Node{child}})
		}
		return node, nil
	case []any:
		node := artifact.Node{Kind: artifact.KindArray, Name: name, Locator: locator}
		for index, item := range typed {
			child, err := b.node(ctx, item, pointer+"/"+strconv.Itoa(index), strconv.Itoa(index), depth+1)
			if err != nil {
				return artifact.Node{}, err
			}
			node.Children = append(node.Children, child)
		}
		return node, nil
	case nil:
		return scalar(name, "null", "null", locator), nil
	case bool:
		return scalar(name, strconv.FormatBool(typed), "boolean", locator), nil
	case json.Number:
		return scalar(name, typed.String(), "number", locator), nil
	case string:
		return scalar(name, typed, "string", locator), nil
	default:
		return artifact.Node{}, fmt.Errorf("normalize TOML: unsupported value type %T", value)
	}
}

func scalar(name, text, valueType string, locator artifact.Locator) artifact.Node {
	return artifact.Node{Kind: artifact.KindValue, Name: name, Text: text, Locator: locator, Attributes: map[string]string{"type": valueType}}
}

func position(node *yaml.Node) map[string]string {
	attributes := make(map[string]string, 2)
	if node.Line > 0 {
		attributes["line"] = strconv.Itoa(node.Line)
	}
	if node.Column > 0 {
		attributes["column"] = strconv.Itoa(node.Column)
	}
	return attributes
}

func yamlType(tag string) string {
	switch tag {
	case "!!null":
		return "null"
	case "!!bool":
		return "boolean"
	case "!!int", "!!float":
		return "number"
	case "!!timestamp":
		return "timestamp"
	default:
		return "string"
	}
}

func escapePointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}
