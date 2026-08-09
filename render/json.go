// Package render serializes normalized artifacts for humans and agents.
package render

import (
	"encoding/json"
	"io"

	"github.com/salman0ansari/artifactkit/artifact"
)

func JSON(writer io.Writer, document *artifact.Artifact, pretty bool) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(document)
}
