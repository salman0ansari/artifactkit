package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/salman0ansari/artifactkit/artifact"
)

// Markdown writes a compact semantic view of an artifact.
func Markdown(writer io.Writer, document *artifact.Artifact) error {
	if document == nil {
		return nil
	}
	if _, err := fmt.Fprintf(writer, "# %s\n\n", document.Name); err != nil {
		return err
	}
	for i := range document.Root.Children {
		if err := markdownNode(writer, &document.Root.Children[i], 2, 0); err != nil {
			return err
		}
	}
	return nil
}

func markdownNode(writer io.Writer, node *artifact.Node, headingLevel, indent int) error {
	switch node.Kind {
	case artifact.KindSection, artifact.KindSlide, artifact.KindSheet:
		level := headingLevel
		if node.Level > 0 {
			level = node.Level + 1
		}
		if level > 6 {
			level = 6
		}
		if _, err := fmt.Fprintf(writer, "%s %s\n\n", strings.Repeat("#", level), node.Name); err != nil {
			return err
		}
	case artifact.KindParagraph:
		if _, err := fmt.Fprintf(writer, "%s\n\n", node.Text); err != nil {
			return err
		}
	case artifact.KindTable:
		if err := markdownTable(writer, node); err != nil {
			return err
		}
	case artifact.KindAttachment:
		if _, err := fmt.Fprintf(writer, "- Attachment: **%s**%s\n", node.Name, detailSuffix(node.Attributes)); err != nil {
			return err
		}
	case artifact.KindEntry:
		if _, err := fmt.Fprintf(writer, "- `%s`%s\n", node.Name, detailSuffix(node.Attributes)); err != nil {
			return err
		}
	case artifact.KindLink:
		label := node.Name
		if label == "" {
			label = node.Attributes["href"]
		}
		if _, err := fmt.Fprintf(writer, "[%s](%s)\n\n", label, node.Attributes["href"]); err != nil {
			return err
		}
	case artifact.KindImage:
		if source := node.Attributes["src"]; source != "" {
			if _, err := fmt.Fprintf(writer, "![%s](%s)\n\n", node.Name, source); err != nil {
				return err
			}
		} else if _, err := fmt.Fprintf(writer, "- Image: **%s**%s\n", node.Name, detailSuffix(node.Attributes)); err != nil {
			return err
		}
	case artifact.KindField:
		if len(node.Children) == 1 && node.Children[0].Kind == artifact.KindValue {
			if _, err := fmt.Fprintf(writer, "%s- **%s:** %s\n", strings.Repeat("  ", indent), node.Name, node.Children[0].Text); err != nil {
				return err
			}
			return nil
		}
		if _, err := fmt.Fprintf(writer, "%s- **%s**\n", strings.Repeat("  ", indent), node.Name); err != nil {
			return err
		}
		indent++
	case artifact.KindValue:
		if _, err := fmt.Fprintf(writer, "%s- %s\n", strings.Repeat("  ", indent), node.Text); err != nil {
			return err
		}
	case artifact.KindObject, artifact.KindArray:
		if node.Name != "" && node.Name != "root" {
			if _, err := fmt.Fprintf(writer, "%s- **%s**\n", strings.Repeat("  ", indent), node.Name); err != nil {
				return err
			}
			indent++
		}
	default:
		if node.Text != "" {
			if _, err := fmt.Fprintf(writer, "%s\n\n", node.Text); err != nil {
				return err
			}
		}
	}
	for i := range node.Children {
		if err := markdownNode(writer, &node.Children[i], headingLevel+1, indent); err != nil {
			return err
		}
	}
	return nil
}

func detailSuffix(attributes map[string]string) string {
	var details []string
	for _, key := range []string{"media_type", "size", "width", "height", "type"} {
		if value := attributes[key]; value != "" {
			details = append(details, key+"="+value)
		}
	}
	if len(details) == 0 {
		return ""
	}
	return " (" + strings.Join(details, ", ") + ")"
}

func markdownTable(writer io.Writer, table *artifact.Node) error {
	if len(table.Children) == 0 {
		return nil
	}
	width := 0
	for _, row := range table.Children {
		if len(row.Children) > width {
			width = len(row.Children)
		}
	}
	writeRow := func(row *artifact.Node) error {
		cells := make([]string, width)
		for i := 0; i < len(row.Children) && i < width; i++ {
			cells[i] = strings.ReplaceAll(strings.ReplaceAll(row.Children[i].Text, "|", "\\|"), "\n", " ")
		}
		_, err := fmt.Fprintf(writer, "| %s |\n", strings.Join(cells, " | "))
		return err
	}
	if err := writeRow(&table.Children[0]); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "| %s |\n", strings.TrimSuffix(strings.Repeat("--- | ", width), " | ")); err != nil {
		return err
	}
	for i := 1; i < len(table.Children); i++ {
		if err := writeRow(&table.Children[i]); err != nil {
			return err
		}
	}
	_, err := io.WriteString(writer, "\n")
	return err
}
