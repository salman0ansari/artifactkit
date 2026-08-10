package main

import (
	"context"
	"io"
	"os"

	"github.com/salman0ansari/artifactkit/internal/cli"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return cli.Run(ctx, args, stdin, stdout, stderr)
}
