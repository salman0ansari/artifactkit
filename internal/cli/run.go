package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	artifactkit "github.com/salman0ansari/artifactkit"
	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/render"
	"github.com/salman0ansari/artifactkit/search"
)

func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "inspect":
		return inspect(ctx, args[1:], stdin, stdout, stderr)
	case "find":
		return find(ctx, args[1:], stdin, stdout, stderr)
	case "formats":
		return formats(args[1:], stdout, stderr)
	case "version", "--version", "-version":
		fmt.Fprintln(stdout, artifactkit.Version)
		return 0
	case "help", "--help", "-h":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "artifactkit: unknown command %q\n\n", args[0])
		usage(stderr)
		return 2
	}
}

func inspect(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("inspect", flag.ContinueOnError)
	set.SetOutput(stderr)
	output := set.String("output", "json", "output format: json or markdown")
	pretty := set.Bool("pretty", false, "pretty-print JSON")
	name := set.String("name", "stdin.txt", "name to use when reading stdin")
	maxBytes := set.Int64("max-bytes", artifact.DefaultLimits().MaxInputBytes, "maximum input bytes")
	if err := set.Parse(args); err != nil {
		return 2
	}
	if set.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: artifactkit inspect [flags] FILE")
		return 2
	}
	engine, err := configuredEngine(*maxBytes)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	document, err := load(ctx, engine, set.Arg(0), *name, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	switch *output {
	case "json":
		err = render.JSON(stdout, document, *pretty)
	case "markdown", "md":
		err = render.Markdown(stdout, document)
	default:
		fmt.Fprintf(stderr, "artifactkit: unsupported output %q\n", *output)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "artifactkit: write output: %v\n", err)
		return 1
	}
	return 0
}

func find(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("find", flag.ContinueOnError)
	set.SetOutput(stderr)
	limit := set.Int("limit", 20, "maximum results")
	name := set.String("name", "stdin.txt", "name to use when reading stdin")
	maxBytes := set.Int64("max-bytes", artifact.DefaultLimits().MaxInputBytes, "maximum input bytes")
	if err := set.Parse(args); err != nil {
		return 2
	}
	if set.NArg() < 2 {
		fmt.Fprintln(stderr, "usage: artifactkit find [flags] FILE QUERY")
		return 2
	}
	engine, err := configuredEngine(*maxBytes)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	document, err := load(ctx, engine, set.Arg(0), *name, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	results := search.Find(document, strings.Join(set.Args()[1:], " "), *limit)
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(results); err != nil {
		fmt.Fprintf(stderr, "artifactkit: write output: %v\n", err)
		return 1
	}
	return 0
}

func formats(args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("formats", flag.ContinueOnError)
	set.SetOutput(stderr)
	asJSON := set.Bool("json", false, "emit JSON")
	if err := set.Parse(args); err != nil {
		return 2
	}
	if set.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: artifactkit formats [--json]")
		return 2
	}
	formats := artifactkit.New().Formats()
	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(formats); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	for _, format := range formats {
		fmt.Fprintf(stdout, "%-12s %-36s %s\n", format.Name, format.MediaType, strings.Join(format.Extensions, ","))
	}
	return 0
}

func configuredEngine(maxBytes int64) (*artifactkit.Engine, error) {
	if maxBytes <= 0 {
		return nil, errors.New("artifactkit: --max-bytes must be positive")
	}
	limits := artifact.DefaultLimits()
	limits.MaxInputBytes = maxBytes
	return artifactkit.New(artifactkit.WithLimits(limits)), nil
}

func load(ctx context.Context, engine *artifactkit.Engine, path, name string, stdin io.Reader) (*artifact.Artifact, error) {
	if path == "-" {
		return engine.InspectReader(ctx, name, stdin)
	}
	return engine.InspectPath(ctx, path)
}

func usage(writer io.Writer) {
	fmt.Fprintln(writer, "ArtifactKit — local artifacts as navigable trees for agents")
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "Usage:")
	fmt.Fprintln(writer, "  artifactkit inspect [--output json|markdown] [--pretty] FILE")
	fmt.Fprintln(writer, "  artifactkit find [--limit N] FILE QUERY")
	fmt.Fprintln(writer, "  artifactkit formats [--json]")
	fmt.Fprintln(writer, "  artifactkit version")
}
