package artifact

import (
	"strings"
	"testing"
)

func TestResourceURIIsStableAndEscaped(t *testing.T) {
	first := ResourceURI([]byte("source"), "entry", "docs/report one.txt")
	second := ResourceURI([]byte("source"), "entry", "docs/report one.txt")
	if first != second {
		t.Fatalf("URI is not stable: %q != %q", first, second)
	}
	if !strings.HasPrefix(first, "artifact://") || strings.Contains(first, " ") {
		t.Fatalf("invalid resource URI: %q", first)
	}
}
