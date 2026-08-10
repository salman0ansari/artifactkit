# MCP server

Run ArtifactKit as a local stdio MCP server:

```bash
artifactkit mcp --root /absolute/path/to/documents
```

Repeat `--root` to authorize multiple directories. With no flag, only the current directory is available. Paths are resolved through symlinks before the root check.

A generic client configuration looks like:

```json
{
  "mcpServers": {
    "artifactkit": {
      "command": "/absolute/path/to/artifactkit",
      "args": ["mcp", "--root", "/absolute/path/to/documents"]
    }
  }
}
```

## Tools

| Tool | Purpose |
| --- | --- |
| `artifact_inspect` | detect a file and return its artifact ID, metadata, and bounded outline |
| `artifact_inspect_resource` | parse an attachment or archive entry without writing a temporary file |
| `artifact_find` | search semantic nodes by token query |
| `artifact_read` | read a node by characters or a lazy resource by bytes |
| `artifact_extract_table` | page through a table while preserving cell IDs and formulas |
| `artifact_list_attachments` | list email attachments, archive entries, and embedded Office media |
| `artifact_get_provenance` | return a node's exact locator and semantic ancestry |
| `artifact_formats` | list formats supported by the running build |

`artifact_inspect` accepts a path. `artifact_inspect_resource` accepts a URI returned by `artifact_list_attachments` and applies the same input and parser limits to the extracted bytes. Subsequent calls can use the returned `artifact_id`, avoiding repeated parse work. Loaded artifacts and source bytes are separately LRU-bounded.

All tools are declared read-only, idempotent, and closed-world. ArtifactKit does not make network requests on their behalf.
