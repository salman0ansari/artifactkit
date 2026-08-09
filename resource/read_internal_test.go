package resource

import (
	"context"
	"strings"
	"testing"
)

func TestReadRangeRejectsTruncatedContent(t *testing.T) {
	_, err := readRange(context.Background(), strings.NewReader("short"), 10, 0, 10)
	if err == nil {
		t.Fatal("expected an error for content shorter than its declared size")
	}
}
