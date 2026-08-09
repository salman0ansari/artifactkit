// Package web parses HTML and XML into semantic artifact trees.
package web

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/salman0ansari/artifactkit/artifact"
	"github.com/salman0ansari/artifactkit/parser"
)

type HTMLParser struct{}
type XMLParser struct{}

func NewHTML() *HTMLParser { return &HTMLParser{} }
func NewXML() *XMLParser   { return &XMLParser{} }

func (*HTMLParser) Name() string { return "html" }
func (*HTMLParser) Formats() []artifact.Format {
	return []artifact.Format{{Name: "html", MediaType: "text/html", Extensions: []string{".html", ".htm"}}}
}
func (*HTMLParser) Probe(source parser.Source) parser.Match {
	trimmed := bytes.TrimSpace(source.Data)
	lower := bytes.ToLower(trimmed)
	if bytes.HasPrefix(lower, []byte("<!doctype html")) || bytes.HasPrefix(lower, []byte("<html")) {
		return parser.Match{Score: 100, Reason: "HTML document signature"}
	}
	if source.Extension() == ".html" || source.Extension() == ".htm" {
		return parser.Match{Score: 90, Reason: "HTML extension"}
	}
	return parser.Match{}
}

func (*HTMLParser) Parse(ctx context.Context, source parser.Source, limits artifact.Limits) (*artifact.Artifact, error) {
	dom, err := html.Parse(bytes.NewReader(source.Data))
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}
	builder := htmlBuilder{ctx: ctx, limits: limits}
	root := artifact.Node{Kind: artifact.KindDocument, Name: source.Name}
	body := findElement(dom, "body")
	if body == nil {
		body = dom
	}
	if err := builder.walk(body, &root); err != nil {
		return nil, err
	}
	result := &artifact.Artifact{Name: source.Name, Format: "html", MediaType: "text/html", Root: root}
	if title := findElement(dom, "title"); title != nil {
		if value := normalizedText(title); value != "" {
			result.Metadata = map[string]string{"title": value}
		}
	}
	return result, nil
}

type htmlBuilder struct {
	ctx    context.Context
	limits artifact.Limits
	nodes  int
}

func (b *htmlBuilder) walk(parent *html.Node, target *artifact.Node) error {
	for current := parent.FirstChild; current != nil; current = current.NextSibling {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		if current.Type != html.ElementNode {
			continue
		}
		tag := strings.ToLower(current.Data)
		if tag == "script" || tag == "style" || tag == "noscript" || tag == "template" {
			continue
		}
		var node *artifact.Node
		switch tag {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			level, _ := strconv.Atoi(tag[1:])
			node = &artifact.Node{Kind: artifact.KindSection, Name: normalizedText(current), Level: level, Locator: artifact.Locator{Path: domPath(current)}}
		case "p", "blockquote", "pre", "figcaption":
			text := normalizedText(current)
			if text != "" {
				node = &artifact.Node{Kind: artifact.KindParagraph, Text: text, Locator: artifact.Locator{Path: domPath(current)}, Attributes: map[string]string{"element": tag}}
			}
		case "li":
			text := normalizedText(current)
			if text != "" {
				node = &artifact.Node{Kind: artifact.KindParagraph, Text: text, Locator: artifact.Locator{Path: domPath(current)}, Attributes: map[string]string{"element": "list-item"}}
			}
		case "table":
			table, err := b.table(current)
			if err != nil {
				return err
			}
			node = &table
		case "img":
			node = &artifact.Node{Kind: artifact.KindImage, Name: attribute(current, "alt"), Locator: artifact.Locator{Path: domPath(current)}, Attributes: compactAttributes(current, "src", "alt", "width", "height")}
		case "a":
			if href := attribute(current, "href"); href != "" {
				node = &artifact.Node{Kind: artifact.KindLink, Name: normalizedText(current), Locator: artifact.Locator{Path: domPath(current)}, Attributes: map[string]string{"href": href}}
			}
		}
		if node != nil {
			if err := b.add(target, *node); err != nil {
				return err
			}
			if tag == "p" || tag == "li" || tag == "table" || tag == "a" || tag == "pre" || tag == "blockquote" || tag == "figcaption" {
				continue
			}
		}
		if err := b.walk(current, target); err != nil {
			return err
		}
	}
	return nil
}

func (b *htmlBuilder) table(element *html.Node) (artifact.Node, error) {
	table := artifact.Node{Kind: artifact.KindTable, Locator: artifact.Locator{Path: domPath(element)}}
	var visit func(*html.Node) error
	visit = func(current *html.Node) error {
		if current.Type == html.ElementNode && strings.EqualFold(current.Data, "tr") {
			row := artifact.Node{Kind: artifact.KindRow, Name: strconv.Itoa(len(table.Children) + 1), Locator: artifact.Locator{Path: domPath(current)}}
			for cellElement := current.FirstChild; cellElement != nil; cellElement = cellElement.NextSibling {
				if cellElement.Type != html.ElementNode || (cellElement.Data != "td" && cellElement.Data != "th") {
					continue
				}
				cellID := spreadsheetCell(len(row.Children)+1, len(table.Children)+1)
				cell := artifact.Node{Kind: artifact.KindCell, Name: cellID, Text: normalizedText(cellElement), Locator: artifact.Locator{Path: domPath(cellElement), Cell: cellID}}
				if cellElement.Data == "th" {
					cell.Attributes = map[string]string{"role": "header"}
				}
				row.Children = append(row.Children, cell)
			}
			if len(row.Children) > 0 {
				table.Children = append(table.Children, row)
				b.nodes += 1 + len(row.Children)
				if b.nodes > b.limits.MaxNodes {
					return &artifact.LimitError{Limit: "nodes", Value: int64(b.nodes), Max: int64(b.limits.MaxNodes)}
				}
			}
			return nil
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(element); err != nil {
		return artifact.Node{}, err
	}
	return table, nil
}

func (b *htmlBuilder) add(parent *artifact.Node, node artifact.Node) error {
	b.nodes++
	if b.nodes > b.limits.MaxNodes {
		return &artifact.LimitError{Limit: "nodes", Value: int64(b.nodes), Max: int64(b.limits.MaxNodes)}
	}
	parent.Children = append(parent.Children, node)
	return nil
}

func findElement(root *html.Node, name string) *html.Node {
	if root.Type == html.ElementNode && strings.EqualFold(root.Data, name) {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if found := findElement(child, name); found != nil {
			return found
		}
	}
	return nil
}

func normalizedText(root *html.Node) string {
	var parts []string
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.ElementNode {
			tag := strings.ToLower(current.Data)
			if tag == "script" || tag == "style" || tag == "noscript" || tag == "template" {
				return
			}
		}
		if current.Type == html.TextNode {
			parts = append(parts, current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

func domPath(node *html.Node) string {
	var segments []string
	for current := node; current != nil; current = current.Parent {
		if current.Type != html.ElementNode {
			continue
		}
		index := 1
		for sibling := current.PrevSibling; sibling != nil; sibling = sibling.PrevSibling {
			if sibling.Type == html.ElementNode && sibling.Data == current.Data {
				index++
			}
		}
		segments = append([]string{fmt.Sprintf("%s[%d]", current.Data, index)}, segments...)
	}
	return "/" + strings.Join(segments, "/")
}

func attribute(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
}

func compactAttributes(node *html.Node, keys ...string) map[string]string {
	attributes := make(map[string]string)
	for _, key := range keys {
		if value := attribute(node, key); value != "" {
			attributes[key] = value
		}
	}
	if len(attributes) == 0 {
		return nil
	}
	return attributes
}

func spreadsheetCell(column, row int) string {
	var letters []byte
	for column > 0 {
		column--
		letters = append([]byte{byte('A' + column%26)}, letters...)
		column /= 26
	}
	return string(letters) + strconv.Itoa(row)
}

func (*XMLParser) Name() string { return "xml" }
func (*XMLParser) Formats() []artifact.Format {
	return []artifact.Format{{Name: "xml", MediaType: "application/xml", Extensions: []string{".xml"}}}
}
func (*XMLParser) Probe(source parser.Source) parser.Match {
	trimmed := bytes.TrimSpace(source.Data)
	if bytes.HasPrefix(trimmed, []byte("<?xml")) {
		return parser.Match{Score: 100, Reason: "XML declaration"}
	}
	if source.Extension() == ".xml" {
		return parser.Match{Score: 85, Reason: "XML extension"}
	}
	return parser.Match{}
}

func (*XMLParser) Parse(ctx context.Context, source parser.Source, limits artifact.Limits) (*artifact.Artifact, error) {
	decoder := xml.NewDecoder(bytes.NewReader(source.Data))
	root := artifact.Node{Kind: artifact.KindDocument, Name: source.Name}
	type frame struct {
		node     *artifact.Node
		path     string
		children map[string]int
	}
	stack := []frame{{node: &root, children: make(map[string]int)}}
	nodes := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse XML: %w", err)
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if len(stack) > limits.MaxNestingDepth {
				return nil, &artifact.LimitError{Limit: "nesting depth", Value: int64(len(stack)), Max: int64(limits.MaxNestingDepth)}
			}
			parent := &stack[len(stack)-1]
			parent.children[typed.Name.Local]++
			path := parent.path + "/" + typed.Name.Local + "[" + strconv.Itoa(parent.children[typed.Name.Local]) + "]"
			attributes := make(map[string]string)
			for _, attr := range typed.Attr {
				attributes[attr.Name.Local] = attr.Value
			}
			if len(attributes) == 0 {
				attributes = nil
			}
			parent.node.Children = append(parent.node.Children, artifact.Node{Kind: artifact.KindElement, Name: typed.Name.Local, Locator: artifact.Locator{Path: path}, Attributes: attributes})
			child := &parent.node.Children[len(parent.node.Children)-1]
			stack = append(stack, frame{node: child, path: path, children: make(map[string]int)})
			nodes++
			if nodes > limits.MaxNodes {
				return nil, &artifact.LimitError{Limit: "nodes", Value: int64(nodes), Max: int64(limits.MaxNodes)}
			}
		case xml.CharData:
			if len(stack) > 1 {
				value := strings.TrimSpace(string(typed))
				if value != "" {
					current := stack[len(stack)-1].node
					if current.Text == "" {
						current.Text = value
					} else {
						current.Text += " " + value
					}
				}
			}
		case xml.EndElement:
			if len(stack) <= 1 {
				return nil, fmt.Errorf("parse XML: unexpected closing element %s", typed.Name.Local)
			}
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) != 1 || len(root.Children) == 0 {
		return nil, fmt.Errorf("parse XML: incomplete document")
	}
	return &artifact.Artifact{Name: source.Name, Format: "xml", MediaType: "application/xml", Root: root}, nil
}
