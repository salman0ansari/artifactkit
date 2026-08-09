package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
)

// ResourceURI builds a stable URI for content contained by an artifact.
func ResourceURI(source []byte, resourceKind, logicalPath string) string {
	digest := sha256.Sum256(source)
	cleanKind := strings.Trim(strings.ReplaceAll(resourceKind, "/", "-"), " ")
	if cleanKind == "" {
		cleanKind = "resource"
	}
	return "artifact://" + hex.EncodeToString(digest[:]) + "/" + cleanKind + "/" + url.PathEscape(logicalPath)
}
