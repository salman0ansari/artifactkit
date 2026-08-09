// Package mcpserver exposes ArtifactKit through the official Model Context Protocol Go SDK.
package mcpserver

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	artifactkit "github.com/salman0ansari/artifactkit"
	"github.com/salman0ansari/artifactkit/agent"
	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/resource"
	"github.com/salman0ansari/artifactkit/search"
)

type InspectInput struct {
	Path string `json:"path" jsonschema:"Path to a local artifact under a configured root."`
}

type ReferenceInput struct {
	Artifact string `json:"artifact" jsonschema:"Artifact ID returned by artifact_inspect, or a local path under a configured root."`
}

type FindInput struct {
	Artifact string `json:"artifact" jsonschema:"Artifact ID or local path."`
	Query    string `json:"query" jsonschema:"Case-insensitive search query; every token must match."`
	Limit    int    `json:"limit,omitempty" jsonschema:"Maximum results; defaults to 20."`
}

type ReadInput struct {
	Artifact    string `json:"artifact,omitempty" jsonschema:"Artifact ID or path; required for node reads."`
	NodeID      string `json:"node_id,omitempty" jsonschema:"Node ID to read. Set either node_id or resource_uri."`
	ResourceURI string `json:"resource_uri,omitempty" jsonschema:"Lazy resource URI from artifact_list_attachments. Set either resource_uri or node_id."`
	Offset      int64  `json:"offset,omitempty" jsonschema:"Character offset for nodes or byte offset for resources."`
	Limit       int64  `json:"limit,omitempty" jsonschema:"Characters or bytes to return; a safe default is used when omitted."`
}

type ReadOutput struct {
	Kind     string          `json:"kind"`
	Node     *agent.NodeView `json:"node,omitempty"`
	Resource *ResourceView   `json:"resource,omitempty"`
}

type ResourceView struct {
	URI       string `json:"uri"`
	Name      string `json:"name"`
	MediaType string `json:"media_type,omitempty"`
	Size      int64  `json:"size"`
	Offset    int64  `json:"offset"`
	EOF       bool   `json:"eof"`
	Encoding  string `json:"encoding"`
	Content   string `json:"content"`
}

type TableInput struct {
	Artifact string `json:"artifact" jsonschema:"Artifact ID or local path."`
	Table    string `json:"table,omitempty" jsonschema:"Table or sheet node ID, table name, or blank for the first table."`
	Offset   int    `json:"offset,omitempty" jsonschema:"Zero-based data-row offset, excluding the header row."`
	Limit    int    `json:"limit,omitempty" jsonschema:"Maximum data rows; defaults to 50 and cannot exceed 500."`
}

type ProvenanceInput struct {
	Artifact string `json:"artifact" jsonschema:"Artifact ID or local path."`
	NodeID   string `json:"node_id" jsonschema:"Deterministic node ID."`
}

func New(service *agent.Service, version string) *mcp.Server {
	if version == "" {
		version = "dev"
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "artifactkit", Version: version}, nil)
	annotations := &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: boolPointer(false)}

	mcp.AddTool(server, &mcp.Tool{
		Name: "artifact_inspect", Title: "Inspect artifact",
		Description: "Detect and parse a local artifact into a bounded typed tree; returns an artifact ID and concise outline.", Annotations: annotations,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input InspectInput) (*mcp.CallToolResult, agent.Summary, error) {
		document, err := service.Inspect(ctx, input.Path)
		if err != nil {
			return nil, agent.Summary{}, err
		}
		return nil, agent.Summarize(document), nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "artifact_find", Title: "Find in artifact",
		Description: "Search semantic nodes without loading the entire artifact into model context.", Annotations: annotations,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input FindInput) (*mcp.CallToolResult, []search.Result, error) {
		results, err := service.Find(ctx, input.Artifact, input.Query, input.Limit)
		return nil, results, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "artifact_read", Title: "Read artifact content",
		Description: "Read a bounded character range from a node or byte range from an attachment/archive/Office resource.", Annotations: annotations,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ReadInput) (*mcp.CallToolResult, ReadOutput, error) {
		if (input.NodeID == "") == (input.ResourceURI == "") {
			return nil, ReadOutput{}, fmt.Errorf("set exactly one of node_id or resource_uri")
		}
		if input.NodeID != "" {
			if input.Artifact == "" {
				return nil, ReadOutput{}, fmt.Errorf("artifact is required for a node read")
			}
			if input.Offset > int64(^uint(0)>>1) || input.Limit > int64(^uint(0)>>1) {
				return nil, ReadOutput{}, fmt.Errorf("node range is too large")
			}
			node, err := service.ReadNode(ctx, input.Artifact, input.NodeID, int(input.Offset), int(input.Limit))
			return nil, ReadOutput{Kind: "node", Node: node}, err
		}
		content, err := service.Engine().ReadResource(ctx, input.ResourceURI, input.Offset, input.Limit)
		if err != nil {
			return nil, ReadOutput{}, err
		}
		return nil, ReadOutput{Kind: "resource", Resource: resourceView(content)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "artifact_extract_table", Title: "Extract table",
		Description: "Return a bounded row window with cell IDs, values, formulas, and exact provenance.", Annotations: annotations,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input TableInput) (*mcp.CallToolResult, agent.TableResult, error) {
		table, err := service.ExtractTable(ctx, input.Artifact, input.Table, input.Offset, input.Limit)
		if err != nil {
			return nil, agent.TableResult{}, err
		}
		return nil, *table, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "artifact_list_attachments", Title: "List attachments",
		Description: "List email attachments, archive entries, and embedded Office media available through lazy resource URIs.", Annotations: annotations,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ReferenceInput) (*mcp.CallToolResult, []artifact.Resource, error) {
		resources, err := service.Resources(ctx, input.Artifact)
		return nil, resources, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "artifact_get_provenance", Title: "Get provenance",
		Description: "Return the exact source locator and semantic ancestry for a node.", Annotations: annotations,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ProvenanceInput) (*mcp.CallToolResult, agent.Provenance, error) {
		value, err := service.Provenance(ctx, input.Artifact, input.NodeID)
		if err != nil {
			return nil, agent.Provenance{}, err
		}
		return nil, *value, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "artifact_formats", Title: "List formats",
		Description: "List every artifact format supported by this ArtifactKit build.", Annotations: annotations,
	}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, []artifact.Format, error) {
		return nil, service.Engine().Formats(), nil
	})

	return server
}

func Run(ctx context.Context, roots []string, reader io.Reader, writer io.Writer) error {
	engine := artifactkit.New()
	service, err := agent.NewService(engine, agent.Options{Roots: roots})
	if err != nil {
		return err
	}
	transport := &mcp.IOTransport{Reader: readCloser{Reader: reader}, Writer: writeCloser{Writer: writer}}
	return New(service, artifactkit.Version).Run(ctx, transport)
}

func resourceView(content *resource.Content) *ResourceView {
	view := &ResourceView{
		URI: content.URI, Name: content.Name, MediaType: content.MediaType, Size: content.Size,
		Offset: content.Offset, EOF: content.EOF, Encoding: "base64",
	}
	if isTextual(content.MediaType) && utf8.Valid(content.Data) {
		view.Encoding = "utf-8"
		view.Content = string(content.Data)
	} else {
		view.Content = base64.StdEncoding.EncodeToString(content.Data)
	}
	return view
}

func isTextual(mediaType string) bool {
	return strings.HasPrefix(mediaType, "text/") || mediaType == "application/json" || mediaType == "application/xml" || strings.HasSuffix(mediaType, "+xml")
}

func boolPointer(value bool) *bool { return &value }

type readCloser struct{ io.Reader }

func (readCloser) Close() error { return nil }

type writeCloser struct{ io.Writer }

func (writeCloser) Close() error { return nil }
