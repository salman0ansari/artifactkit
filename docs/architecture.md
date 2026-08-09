# Architecture

ArtifactKit separates format parsing from agent operations so every interface has the same behavior.

```text
local file / bytes
        |
        v
 bounded read -> confidence-based registry -> format parser
                                           |
                                           v
                             deterministic artifact tree
                               |       |         |
                               v       v         v
                            search   render   provenance
                               |                 |
                               +-------+---------+
                                       v
                              agent service / MCP

 embedded bytes -> bounded LRU source store -> lazy range reader
```

## Packages

- `artifact` defines nodes, locators, resources, warnings, and limits.
- `parser` defines immutable sources and the confidence-ranked parser registry.
- `parsers/*` contains format adapters. OOXML shares a safe ZIP reader but has distinct DOCX, PPTX, and XLSX semantics.
- `resource` retains only source bytes needed by lazy resources and evicts them with an LRU bound.
- `search`, `render`, and `agent` operate only on normalized artifacts.
- `mcpserver` maps the agent service to the official MCP Go SDK.

## Determinism

Artifact IDs are SHA-256 digests of input bytes. Node IDs combine that digest with semantic kind and traversal path. Map-backed formats such as JSON are sorted before node creation. Fixture archives use fixed timestamps and entry ordering.

## Resource lifetime

The default engine retains source bytes only for artifacts with attachments or embedded parts. The cache is bounded by bytes and artifact count. A resource URI remains stable, but reads return an expiration error after its source is evicted. Applications can provide a shared store or disable storage with `artifactkit.WithResourceStore(nil)`.

## Trust boundary

Parsers receive bytes only after input limits are checked. Container formats are preflighted for paths, duplicate entries, expansion, entry count, and compression ratio before XML or embedded content is opened. The MCP service additionally resolves symlinks and restricts reads to configured roots.

