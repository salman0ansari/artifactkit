package mcpserver

import (
	"context"
	"path/filepath"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	artifactkit "github.com/salman0ansari/artifactkit"
	"github.com/salman0ansari/artifactkit/agent"
)

func TestMCPServerAgentWorkflow(t *testing.T) {
	ctx := context.Background()
	root, err := filepath.Abs(filepath.Join("..", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := agent.NewService(artifactkit.New(), agent.Options{Roots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	server := New(service, "test")
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "artifactkit-test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || !tool.Annotations.IdempotentHint {
			t.Fatalf("missing safe annotations on %s: %#v", tool.Name, tool.Annotations)
		}
	}
	sort.Strings(names)
	if len(names) != 7 || names[0] != "artifact_extract_table" || names[6] != "artifact_read" {
		t.Fatalf("unexpected MCP tools: %v", names)
	}

	inspect := callTool(t, ctx, clientSession, "artifact_inspect", map[string]any{"path": filepath.Join(root, "sample.zip")})
	artifactID := inspect.(map[string]any)["artifact_id"].(string)
	if artifactID == "" || inspect.(map[string]any)["format"] != "zip" {
		t.Fatalf("unexpected inspect result: %#v", inspect)
	}

	resources := callTool(t, ctx, clientSession, "artifact_list_attachments", map[string]any{"artifact": artifactID}).([]any)
	if len(resources) != 2 {
		t.Fatalf("unexpected resources: %#v", resources)
	}
	resourceURI := resources[0].(map[string]any)["uri"].(string)
	read := callTool(t, ctx, clientSession, "artifact_read", map[string]any{"resource_uri": resourceURI, "limit": 12}).(map[string]any)
	resourceView := read["resource"].(map[string]any)
	if read["kind"] != "resource" || resourceView["encoding"] != "utf-8" || resourceView["content"] != "ArtifactKit " {
		t.Fatalf("unexpected resource read: %#v", read)
	}

	workbook := callTool(t, ctx, clientSession, "artifact_inspect", map[string]any{"path": filepath.Join(root, "workbook.xlsx")}).(map[string]any)
	table := callTool(t, ctx, clientSession, "artifact_extract_table", map[string]any{"artifact": workbook["artifact_id"], "table": "Agents"}).(map[string]any)
	if table["total_rows"] != float64(1) {
		t.Fatalf("unexpected table result: %#v", table)
	}
}

func callTool(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, arguments map[string]any) any {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("tool %s returned error: %#v", name, result.Content)
	}
	return result.StructuredContent
}
