// Package ziputil opens hostile ZIP containers with shared ArtifactKit limits.
package ziputil

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"math"
	"path"
	"strings"

	"github.com/salman0ansari/artifactkit/artifact"
)

type Entry struct {
	Name string
	File *zip.File
}

type Reader struct {
	Entries  []Entry
	Expanded int64
	files    map[string]*zip.File
}

func Open(data []byte, limits artifact.Limits) (*Reader, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	if len(archive.File) > limits.MaxArchiveEntries {
		return nil, &artifact.LimitError{Limit: "archive entries", Value: int64(len(archive.File)), Max: int64(limits.MaxArchiveEntries)}
	}
	result := &Reader{Entries: make([]Entry, 0, len(archive.File)), files: make(map[string]*zip.File, len(archive.File))}
	for _, file := range archive.File {
		name, err := CleanPath(file.Name)
		if err != nil {
			return nil, err
		}
		if _, exists := result.files[name]; exists {
			return nil, fmt.Errorf("duplicate archive path %q", name)
		}
		if file.UncompressedSize64 > math.MaxInt64 {
			return nil, &artifact.LimitError{Limit: "expanded bytes", Value: math.MaxInt64, Max: limits.MaxExpandedBytes}
		}
		size := int64(file.UncompressedSize64)
		if size > limits.MaxExpandedBytes-result.Expanded {
			return nil, &artifact.LimitError{Limit: "expanded bytes", Value: result.Expanded + size, Max: limits.MaxExpandedBytes}
		}
		if err := CheckRatio(size, int64(file.CompressedSize64), limits); err != nil {
			return nil, fmt.Errorf("entry %q: %w", name, err)
		}
		result.Expanded += size
		result.Entries = append(result.Entries, Entry{Name: name, File: file})
		result.files[name] = file
	}
	return result, nil
}

func (r *Reader) Has(name string) bool {
	_, exists := r.files[name]
	return exists
}

func (r *Reader) File(name string) (*zip.File, bool) {
	file, exists := r.files[name]
	return file, exists
}

func (r *Reader) Read(name string) ([]byte, error) {
	file, exists := r.files[name]
	if !exists {
		return nil, fmt.Errorf("ZIP part %q not found", name)
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	size := int64(file.UncompressedSize64)
	data, err := io.ReadAll(io.LimitReader(reader, size+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > size {
		return nil, fmt.Errorf("ZIP part %q exceeded its declared size", name)
	}
	return data, nil
}

func CleanPath(value string) (string, error) {
	value = strings.ReplaceAll(value, "\\", "/")
	if value == "" || strings.ContainsRune(value, 0) || strings.HasPrefix(value, "/") || (len(value) >= 2 && value[1] == ':') {
		return "", fmt.Errorf("unsafe archive path %q", value)
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("unsafe archive path %q", value)
	}
	return cleaned, nil
}

func CheckRatio(expanded, compressed int64, limits artifact.Limits) error {
	if expanded <= 0 {
		return nil
	}
	if compressed <= 0 {
		compressed = 1
	}
	ratio := float64(expanded) / float64(compressed)
	if ratio > limits.MaxCompressionRatio {
		return &artifact.LimitError{Limit: "compression ratio", Value: int64(ratio), Max: int64(limits.MaxCompressionRatio)}
	}
	return nil
}
