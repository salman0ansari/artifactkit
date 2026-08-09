# ArtifactKit

ArtifactKit turns local files into deterministic, typed trees that agents can inspect, search, and cite without loading an entire artifact into context.

```text
file -> detect -> parse -> normalized tree -> search / Markdown / JSON / MCP
                              |
                              +-> exact source locator
```

The important output is not another Markdown blob. Every section, value, row, and cell has a stable node ID and provenance such as a byte range, JSON Pointer, or spreadsheet-style cell reference.

## Current formats

| Format | Structure | Provenance |
| --- | --- | --- |
| Markdown | sections and paragraphs | byte ranges |
| Plain text | paragraphs | byte ranges |
| JSON | objects, arrays, fields, values | JSON Pointer |
| CSV | table, rows, cells | row byte ranges and cell IDs |
| TSV | table, rows, cells | row byte ranges and cell IDs |
| HTML | headings, paragraphs, tables, links, images | DOM paths |
| XML | elements, attributes, text | indexed element paths |
| EML | headers, text bodies, attachments | MIME part paths |
| PNG, JPEG, GIF | dimensions and color model | source byte range |
| ZIP, TAR, TAR.GZ, GZIP | safe entry inventory | normalized entry paths |
| DOCX | headings, paragraphs, tables, links, headers, notes | package part and XML subpath |
| PPTX | slides, text, tables, speaker notes | slide, package part, XML subpath |
| XLSX | sheets, rows, cells, formulas, links, merged ranges | sheet, cell, package part |
| PDF | page-scoped text | page number |

## Build

ArtifactKit requires Go 1.24.1 or newer.

```bash
go build -o artifactkit ./cmd/artifactkit
go test ./...
```

## CLI

Inspect a local artifact as structured JSON:

```bash
artifactkit inspect --pretty report.md
```

Render one as compact Markdown:

```bash
artifactkit inspect --output markdown metrics.csv
```

Search without sending the file to a remote service:

```bash
artifactkit find report.md "payment retry"
```

Read from stdin by supplying a synthetic name for detection:

```bash
cat payload.json | artifactkit inspect --name payload.json -
```

List the exact formats supported by the installed build:

```bash
artifactkit formats --json
```

## Go API

```go
package main

import (
    "context"
    "fmt"

    artifactkit "github.com/salman0ansari/artifactkit"
    "github.com/salman0ansari/artifactkit/search"
)

func main() {
    engine := artifactkit.New()
    document, err := engine.InspectPath(context.Background(), "report.md")
    if err != nil {
        panic(err)
    }

    for _, result := range search.Find(document, "payment retry", 10) {
        fmt.Printf("%s %s %s\n", result.NodeID, result.Locator.String(), result.Snippet)
    }
}
```

Use `artifactkit.WithLimits` to set input, text, node, nesting, expansion, and compression-ratio limits. Use `artifactkit.WithRegistry` to run only trusted parsers or register application-specific formats.

## Design rules

- Local first: parsing does not require a network service.
- Agent shaped: bounded search and lazy resource reads are first-class operations.
- Deterministic: identical bytes produce identical artifact and node IDs.
- Provenance preserving: normalized content points back to its source location.
- Safe by default: parsing is bounded; active content and macros are never executed.
- Archive aware: traversal paths, duplicate entries, links, encryption, expansion, and suspicious compression ratios are handled explicitly.
- Embeddable: the CLI and upcoming MCP server use the same public Go engine.

ArtifactKit preserves spreadsheet formulas but never evaluates them. PDF extraction reads embedded text; it does not perform OCR on scanned pages.

## Status

ArtifactKit is under active development. The normalized model is usable now, but compatibility is not yet guaranteed until v1.0.

## License

MIT
