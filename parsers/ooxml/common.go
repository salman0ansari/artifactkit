package ooxml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/salman0ansari/artifactkit/artifact"
)

type relationship struct {
	ID       string
	Type     string
	Target   string
	External bool
}

func (p *packageReader) relationships(ownerPart string) (map[string]relationship, error) {
	relPart := path.Join(path.Dir(ownerPart), "_rels", path.Base(ownerPart)+".rels")
	if !p.archive.Has(relPart) {
		return map[string]relationship{}, nil
	}
	data, err := p.archive.Read(relPart)
	if err != nil {
		return nil, fmt.Errorf("read relationships %s: %w", relPart, err)
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	relationships := make(map[string]relationship)
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("parse relationships %s: %w", relPart, err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "Relationship" {
			continue
		}
		rel := relationship{ID: attr(start, "Id"), Type: attr(start, "Type"), Target: attr(start, "Target"), External: strings.EqualFold(attr(start, "TargetMode"), "External")}
		if rel.ID == "" || rel.Target == "" {
			continue
		}
		if !rel.External {
			rel.Target = resolvePart(ownerPart, rel.Target)
		}
		relationships[rel.ID] = rel
	}
	return relationships, nil
}

func resolvePart(ownerPart, target string) string {
	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(path.Clean(target), "/")
	}
	return path.Clean(path.Join(path.Dir(ownerPart), target))
}

func (p *packageReader) metadata() map[string]string {
	if !p.archive.Has("docProps/core.xml") {
		return nil
	}
	data, err := p.archive.Read("docProps/core.xml")
	if err != nil {
		return nil
	}
	allowed := map[string]string{
		"title": "title", "subject": "subject", "creator": "creator", "description": "description", "keywords": "keywords",
		"lastModifiedBy": "last_modified_by", "revision": "revision", "created": "created", "modified": "modified",
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	metadata := make(map[string]string)
	var key string
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch typed := token.(type) {
		case xml.StartElement:
			key = allowed[typed.Name.Local]
		case xml.CharData:
			if key != "" {
				value := strings.TrimSpace(string(typed))
				if value != "" {
					metadata[key] += value
				}
			}
		case xml.EndElement:
			if _, exists := allowed[typed.Name.Local]; exists {
				key = ""
			}
		}
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func (p *packageReader) packageResources(result *artifact.Artifact, prefixes ...string) {
	for _, entry := range p.archive.Entries {
		name := entry.Name
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, "vbaproject.bin") {
			result.Warnings = append(result.Warnings, artifact.Warning{Code: "macro_ignored", Message: "Office macro project was not executed or exposed", Locator: artifact.Locator{Path: name}})
			continue
		}
		matched := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(name, prefix) {
				matched = true
				break
			}
		}
		if !matched || entry.File.FileInfo().IsDir() {
			continue
		}
		locator := artifact.Locator{Path: name}
		if entry.File.Flags&1 != 0 {
			result.Warnings = append(result.Warnings, artifact.Warning{Code: "encrypted_part_skipped", Message: "encrypted Office package part was not exposed", Locator: locator})
			continue
		}
		if entry.File.Mode()&os.ModeSymlink != 0 {
			result.Warnings = append(result.Warnings, artifact.Warning{Code: "package_link_skipped", Message: "linked Office package part was not exposed", Locator: locator})
			continue
		}
		result.Resources = append(result.Resources, artifact.Resource{
			URI: artifact.ResourceURI(p.source.Data, "part", name), Name: path.Base(name),
			MediaType: officeContentType(name), Size: int64(entry.File.UncompressedSize64), Locator: locator,
		})
	}
	sort.Slice(result.Resources, func(i, j int) bool { return result.Resources[i].Locator.Path < result.Resources[j].Locator.Path })
}

func officeContentType(name string) string {
	return map[string]string{
		".bin": "application/octet-stream", ".csv": "text/csv", ".emf": "image/emf", ".gif": "image/gif", ".jpeg": "image/jpeg",
		".jpg": "image/jpeg", ".pdf": "application/pdf", ".png": "image/png", ".svg": "image/svg+xml", ".tif": "image/tiff", ".tiff": "image/tiff", ".wmf": "image/wmf",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	}[strings.ToLower(path.Ext(name))]
}

func attr(element xml.StartElement, local string) string {
	for _, attribute := range element.Attr {
		if attribute.Name.Local == local {
			return attribute.Value
		}
	}
	return ""
}

func relationshipID(element xml.StartElement) string {
	for _, attribute := range element.Attr {
		if attribute.Name.Local == "id" && strings.Contains(attribute.Name.Space, "relationships") {
			return attribute.Value
		}
	}
	return attr(element, "id")
}

func headingLevel(style string) int {
	lower := strings.ToLower(strings.ReplaceAll(style, " ", ""))
	for _, prefix := range []string{"heading", "title"} {
		if strings.HasPrefix(lower, prefix) {
			value := strings.TrimPrefix(lower, prefix)
			if value == "" && prefix == "title" {
				return 1
			}
			if number, err := strconv.Atoi(value); err == nil && number >= 1 && number <= 9 {
				return number
			}
		}
	}
	return 0
}

func spreadsheetCell(column, row int) string {
	var letters []byte
	for column > 0 {
		column--
		letters = append([]byte{byte('A' + column%26)}, letters...)
		column /= 26
	}
	return string(letters) + strconv.Itoa(row)
}
