package extractor

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/markdown"

	"github.com/kr9ly/skeleton/skeleton"
)

type MarkdownExtractor struct{}

func NewMarkdown() *MarkdownExtractor {
	return &MarkdownExtractor{}
}

func (e *MarkdownExtractor) Extract(src []byte) (*skeleton.File, error) {
	tree, err := markdown.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, err
	}

	file := &skeleton.File{}
	seen := make(map[string]bool)

	// Block tree: extract headings
	root := tree.BlockTree().RootNode()
	mdWalkHeadings(root, src, file)

	// Inline trees: extract local links as imports
	tree.Iter(func(node *markdown.Node) bool {
		if node.Inline == nil {
			return true
		}
		mdExtractLinks(node.Inline, src, file, seen)
		return true
	})

	return file, nil
}

func mdWalkHeadings(node *sitter.Node, src []byte, file *skeleton.File) {
	if node.Type() == "atx_heading" {
		sig, name := mdHeadingText(node, src)
		if name != "" {
			file.Exports = append(file.Exports, skeleton.Export{
				Kind:      skeleton.ExportSection,
				Name:      name,
				Signature: sig,
			})
		}
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		mdWalkHeadings(node.NamedChild(i), src, file)
	}
}

func mdHeadingText(node *sitter.Node, src []byte) (sig, name string) {
	var marker, text string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "atx_h1_marker", "atx_h2_marker", "atx_h3_marker",
			"atx_h4_marker", "atx_h5_marker", "atx_h6_marker":
			marker = content(child, src)
		case "inline":
			text = strings.TrimSpace(content(child, src))
		}
	}
	if text == "" {
		return "", ""
	}
	return marker + " " + text, text
}

func mdExtractLinks(node *sitter.Node, src []byte, file *skeleton.File, seen map[string]bool) {
	if node.Type() == "inline_link" || node.Type() == "shortcut_link" {
		dest := mdLinkDest(node, src)
		if dest != "" && !seen[dest] && isLocalLink(dest) {
			seen[dest] = true
			file.Imports = append(file.Imports, dest)
		}
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		mdExtractLinks(node.NamedChild(i), src, file, seen)
	}
}

func mdLinkDest(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "link_destination" {
			return strings.TrimSpace(content(child, src))
		}
	}
	return ""
}

func isLocalLink(dest string) bool {
	// スキーム付き（https:// 等）やフラグメントのみ（#section）は外部リンク
	return !strings.Contains(dest, "://") && !strings.HasPrefix(dest, "#")
}
