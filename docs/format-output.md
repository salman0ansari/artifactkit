# Format output gallery

ArtifactKit can return a complete typed JSON tree or a compact Markdown view. The examples below show the compact output produced by:

```bash
artifactkit inspect --output markdown FILE
```

The Markdown renderer is useful for prompts and terminal inspection. The default JSON output also includes the artifact ID, digest, media type, stable node IDs, attributes, resources, and exact source locators. For example, an image result has this shape:

```json
{
  "id": "sha256:58de9af2...",
  "name": "pixel.png",
  "format": "png",
  "media_type": "image/png",
  "size": 80,
  "metadata": {
    "height": "3",
    "width": "2"
  },
  "root": {
    "kind": "document",
    "children": [
      {
        "kind": "image",
        "name": "pixel.png",
        "locator": {
          "byte_range": { "start": 0, "end": 80 }
        },
        "attributes": {
          "height": "3",
          "width": "2"
        }
      }
    ]
  }
}
```

IDs and values depend on the input bytes. The examples use small representative files; `...` marks intentionally shortened content.

## Text and structured data

### Markdown

Extensions: `.md`, `.markdown`

Structure: sections and paragraphs with byte-range provenance.

```text
# incident.md

## Incident Review

ArtifactKit parsed the local report without sending it to a remote service.

### Findings

The payment retry worker duplicated three jobs after a lease expired.
```

### Plain text

Extensions: `.txt`, `.text`, `.log`

Structure: paragraphs with byte-range provenance.

```text
# report.txt

Parser ready.

All local checks passed.
```

### JSON

Extension: `.json`

Structure: typed objects, arrays, fields, and values with JSON Pointer locators.

```text
# config.json

- **agent**
  - **agent**
    - **name:** researcher
    - **tools**
      - **tools**
        - inspect
        - find
        - read
- **retries:** 3
- **safe:** true
```

### YAML

Extensions: `.yaml`, `.yml`

Structure: typed mappings, sequences, scalar values, and multi-document streams with key path, line, and column locators.

```text
# settings.yaml

- **agent**
  - **agent**
    - **name:** researcher
    - **enabled:** true
    - **retries:** 3
    - **tags**
      - **tags**
        - local
        - safe
- **models**
  - **models**
    - **primary:** gpt-5
```

### TOML

Extension: `.toml`

Structure: tables, arrays, and typed values with key-path locators.

```text
# pipeline.toml

- **agent**
  - **agent**
    - **enabled:** true
    - **name:** researcher
    - **retries:** 3
- **limits**
  - **limits**
    - **timeout_ms:** 1500
- **title:** ArtifactKit Pipeline
```

### CSV

Extension: `.csv`

Structure: a table containing rows and spreadsheet-addressed cells.

```text
# metrics.csv

| service | latency_ms | status |
| --- | --- | --- |
| parser | 4 | ok |
| indexer | 12 | ok |
| worker | 87 | degraded |
```

### TSV

Extension: `.tsv`

Structure: the same table, row, and cell model as CSV.

```text
# agents.tsv

| agent | status | score |
| --- | --- | --- |
| researcher | ready | 3 |
| reviewer | queued | 1 |
```

### HTML

Extensions: `.html`, `.htm`

Structure: headings, paragraphs, tables, links, and images with DOM-path locators.

```text
# page.html

## Agent Operations

ArtifactKit keeps source provenance close to every extracted value.

| Tool | Status |
| --- | --- |
| inspect | ready |

[Open runbook](https://example.test/runbook)

![Parser pipeline](diagram.png)
```

### XML

Extension: `.xml`

Structure: elements, attributes, and text with indexed element-path locators.

```text
# catalog.xml

Incident report

ready

Metrics

queued
```

### EML

Extension: `.eml`

Structure: message headers, text bodies, and lazy attachments with MIME-part locators.

```text
# message.eml

The agent can inspect this message and list its attachment.

- Attachment: **config.json** (media_type=application/json, size=30)
```

## Images

Image parsers read configuration metadata without decoding the pixel payload.

### PNG

Extension: `.png`

Structure: image dimensions and color model with a source byte range.

```text
# pixel.png

- Image: **pixel.png** (width=2, height=3)
```

### JPEG

Extensions: `.jpg`, `.jpeg`

Structure: image dimensions and color model with a source byte range.

```text
# photo.jpg

- Image: **photo.jpg** (width=1280, height=720)
```

### GIF

Extension: `.gif`

Structure: image dimensions and color model with a source byte range.

```text
# status.gif

- Image: **status.gif** (width=320, height=180)
```

## Archives and compressed files

Archive entries use normalized paths and are exposed as lazy `artifact://` resources. Agents can inspect a nested supported file without extracting it to disk.

### ZIP

Extension: `.zip`

```text
# sample.zip

- `docs/readme.txt` (size=28)
- `data/config.json` (size=33)
```

### TAR

Extension: `.tar`

```text
# sample.tar

- `metrics/status.csv` (size=28)
```

### TAR.GZ

Extensions: `.tar.gz`, `.tgz`

```text
# sample.tar.gz

- `metrics/status.csv` (size=28)
```

### GZIP

Extension: `.gz`

```text
# report.txt.gz

- `report.txt` (size=21)
```

## Documents

### DOCX

Extension: `.docx`

Structure: headings, paragraphs, tables, links, headers, and notes with package-part and XML-path provenance.

```text
# brief.docx

## Agent Brief

ArtifactKit preserves Office provenance and runbook links.

| Tool | Status |
| --- | --- |
| inspect | ready |

### Header

Internal agent report
```

### PPTX

Extension: `.pptx`

Structure: slides, text, tables, and speaker notes with slide and package-part provenance.

```text
# deck.pptx

## Agent Deck

Agent Deck

Local parsing with slide provenance

| Tool | Status |
| --- | --- |
| inspect | ready |

### Speaker notes

Speaker note: cite slide one.
```

### XLSX

Extension: `.xlsx`

Structure: sheets, rows, cells, formulas, links, and merged ranges with sheet and cell locators.

```text
# workbook.xlsx

## Agents

| Agent | Status | Score |
| --- | --- | --- |
| Ready | true | 3 |
```

### PDF

Extension: `.pdf`

Structure: page-scoped embedded text with page-number provenance.

```text
# report.pdf

## Page 1

ArtifactKit PDF fixture - Page provenance is preserved.
```

PDF extraction reads embedded text and does not OCR scanned pages. Spreadsheet formulas are preserved but never evaluated.
