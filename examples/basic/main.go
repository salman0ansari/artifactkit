package main

import (
	"context"
	"fmt"
	"os"

	artifactkit "github.com/salman0ansari/artifactkit"
	"github.com/salman0ansari/artifactkit/search"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./examples/basic FILE")
		os.Exit(2)
	}
	engine := artifactkit.New()
	document, err := engine.InspectPath(context.Background(), os.Args[1])
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s: %s (%d bytes)\n", document.ID, document.Format, document.Size)
	for _, result := range search.Find(document, "status", 10) {
		fmt.Printf("%s %s %s\n", result.NodeID, result.Locator.String(), result.Snippet)
	}
}
