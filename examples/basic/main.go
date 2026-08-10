package main

import (
	"context"
	"fmt"
	"io"
	"os"

	artifactkit "github.com/salman0ansari/artifactkit"
	"github.com/salman0ansari/artifactkit/search"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./examples/basic FILE")
		os.Exit(2)
	}
	if err := run(context.Background(), os.Args[1], os.Stdout); err != nil {
		panic(err)
	}
}

func run(ctx context.Context, path string, writer io.Writer) error {
	engine := artifactkit.New()
	document, err := engine.InspectPath(ctx, path)
	if err != nil {
		return err
	}
	fmt.Fprintf(writer, "%s: %s (%d bytes)\n", document.ID, document.Format, document.Size)
	for _, result := range search.Find(document, "status", 10) {
		fmt.Fprintf(writer, "%s %s %s\n", result.NodeID, result.Locator.String(), result.Snippet)
	}
	return nil
}
