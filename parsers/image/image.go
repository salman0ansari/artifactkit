// Package imageinfo extracts safe metadata from raster images without decoding pixels.
package imageinfo

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strconv"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
)

type Parser struct{}

func New() *Parser           { return &Parser{} }
func (*Parser) Name() string { return "image" }

func (*Parser) Formats() []artifact.Format {
	return []artifact.Format{
		{Name: "gif", MediaType: "image/gif", Extensions: []string{".gif"}},
		{Name: "jpeg", MediaType: "image/jpeg", Extensions: []string{".jpg", ".jpeg"}},
		{Name: "png", MediaType: "image/png", Extensions: []string{".png"}},
	}
}

func (*Parser) Probe(source parser.Source) parser.Match {
	data := source.Data
	switch {
	case len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}):
		return parser.Match{Score: 100, Reason: "PNG signature"}
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return parser.Match{Score: 100, Reason: "JPEG signature"}
	case len(data) >= 6 && (bytes.Equal(data[:6], []byte("GIF87a")) || bytes.Equal(data[:6], []byte("GIF89a"))):
		return parser.Match{Score: 100, Reason: "GIF signature"}
	}
	return parser.Match{}
}

func (*Parser) Parse(ctx context.Context, source parser.Source, _ artifact.Limits) (*artifact.Artifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(source.Data))
	if err != nil {
		return nil, fmt.Errorf("decode image configuration: %w", err)
	}
	mediaType := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif"}[format]
	if mediaType == "" {
		return nil, fmt.Errorf("unsupported decoded image format %q", format)
	}
	attributes := map[string]string{
		"width":       strconv.Itoa(config.Width),
		"height":      strconv.Itoa(config.Height),
		"color_model": fmt.Sprintf("%T", config.ColorModel),
	}
	imageNode := artifact.Node{Kind: artifact.KindImage, Name: source.Name, Locator: artifact.Locator{ByteRange: &artifact.ByteRange{Start: 0, End: int64(len(source.Data))}}, Attributes: attributes}
	return &artifact.Artifact{
		Name: source.Name, Format: format, MediaType: mediaType,
		Metadata: map[string]string{"width": attributes["width"], "height": attributes["height"]},
		Root:     artifact.Node{Kind: artifact.KindDocument, Name: source.Name, Children: []artifact.Node{imageNode}},
	}, nil
}
