//go:build ignore

// Command generate creates deterministic binary fixtures used by parser integration tests.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"image"
	"image/color"
	"image/png"
	"os"
	"time"
)

var fixtureTime = time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)

func main() {
	must(writeZIP("testdata/sample.zip"))
	must(writeTarGZIP("testdata/sample.tar.gz"))
	must(writeGZIP("testdata/report.txt.gz"))
	must(writePNG("testdata/pixel.png"))
}

func writeZIP(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(file)
	entries := []struct{ name, content string }{
		{"docs/readme.txt", "ArtifactKit archive fixture\n"},
		{"data/config.json", `{"agent":"inspector","safe":true}`},
	}
	for _, item := range entries {
		name, content := item.name, item.content
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetModTime(fixtureTime)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return file.Close()
}

func writeTarGZIP(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	gzipWriter := gzip.NewWriter(file)
	gzipWriter.Header.ModTime = fixtureTime
	tarWriter := tar.NewWriter(gzipWriter)
	content := []byte("service,status\nparser,ready\n")
	header := &tar.Header{Name: "metrics/status.csv", Mode: 0o600, Size: int64(len(content)), ModTime: fixtureTime}
	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}
	if _, err := tarWriter.Write(content); err != nil {
		return err
	}
	if err := tarWriter.Close(); err != nil {
		return err
	}
	if err := gzipWriter.Close(); err != nil {
		return err
	}
	return file.Close()
}

func writeGZIP(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	writer := gzip.NewWriter(file)
	writer.Header.Name = "report.txt"
	writer.Header.ModTime = fixtureTime
	if _, err := writer.Write([]byte("bounded gzip fixture\n")); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return file.Close()
}

func writePNG(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	canvas := image.NewRGBA(image.Rect(0, 0, 2, 3))
	canvas.Set(0, 0, color.RGBA{R: 36, G: 120, B: 220, A: 255})
	if err := png.Encode(file, canvas); err != nil {
		return err
	}
	return file.Close()
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
