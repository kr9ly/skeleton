package extractor

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/java"

	"github.com/kr9ly/skeleton/skeleton"
)

type JavaExtractor struct{}

func NewJava() *JavaExtractor {
	return &JavaExtractor{}
}

func (e *JavaExtractor) Extract(src []byte) (*skeleton.File, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(java.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	root := tree.RootNode()
	file := &skeleton.File{}

	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		switch child.Type() {
		case "package_declaration":
			// package com.example; → module identifier, not an import
		case "import_declaration":
			if path := javaImportPath(child, src); path != "" {
				file.Imports = append(file.Imports, path)
			}
		case "class_declaration", "interface_declaration", "enum_declaration",
			"record_declaration", "annotation_type_declaration":
			if javaIsPrivate(child, src) {
				continue
			}
			if exp := javaExtractTypeDecl(child, src); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}
		}
	}

	return file, nil
}

// javaImportPath extracts the dotted path from an import_declaration,
// appending ".*" for wildcard imports.
func javaImportPath(node *sitter.Node, src []byte) string {
	path := ""
	wildcard := false
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "scoped_identifier", "identifier":
			path = content(child, src)
		case "asterisk":
			wildcard = true
		}
	}
	if path == "" {
		return ""
	}
	if wildcard {
		path += ".*"
	}
	return path
}

// javaIsPrivate returns true if the node's modifiers contain private or protected.
func javaIsPrivate(node *sitter.Node, src []byte) bool {
	modifiers := findChildByType(node, "modifiers")
	if modifiers == nil {
		return false
	}
	for i := 0; i < int(modifiers.ChildCount()); i++ {
		switch modifiers.Child(i).Type() {
		case "private", "protected":
			return true
		}
	}
	return false
}

// javaExtractTypeDecl extracts a class/interface/enum/record/annotation declaration.
func javaExtractTypeDecl(node *sitter.Node, src []byte) *skeleton.Export {
	name := node.ChildByFieldName("name")
	if name == nil {
		return nil
	}

	var kind skeleton.ExportKind
	switch node.Type() {
	case "interface_declaration", "annotation_type_declaration":
		kind = skeleton.ExportInterface
	default:
		kind = skeleton.ExportClass
	}

	// Signature: everything up to the body (modifiers, keyword, name,
	// type parameters, extends/implements/permits clauses).
	sig := strings.TrimSpace(content(node, src))
	body := node.ChildByFieldName("body")
	if body != nil {
		sig = strings.TrimSpace(string(src[node.StartByte():body.StartByte()]))
	}

	var members []skeleton.Member
	if body != nil {
		members = javaExtractMembers(body, src)
	}

	start, end := nodeLines(node)
	return &skeleton.Export{
		Kind:      kind,
		Name:      content(name, src),
		Signature: sig,
		Members:   members,
		StartLine: start,
		EndLine:   end,
	}
}

// javaExtractMembers extracts public members from a class/interface/enum/annotation body.
func javaExtractMembers(body *sitter.Node, src []byte) []skeleton.Member {
	var members []skeleton.Member
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		switch child.Type() {
		case "enum_constant":
			if name := child.ChildByFieldName("name"); name != nil {
				n := content(name, src)
				start, end := nodeLines(child)
				members = append(members, skeleton.Member{
					Kind:      skeleton.MemberField,
					Name:      n,
					Signature: n,
					StartLine: start,
					EndLine:   end,
				})
			}
		case "enum_body_declarations":
			members = append(members, javaExtractMembers(child, src)...)
		case "field_declaration", "constant_declaration":
			if javaIsPrivate(child, src) {
				continue
			}
			members = append(members, javaExtractFields(child, src)...)
		case "annotation_type_element_declaration":
			if name := child.ChildByFieldName("name"); name != nil {
				sig := strings.TrimSuffix(strings.TrimSpace(content(child, src)), ";")
				start, end := nodeLines(child)
				members = append(members, skeleton.Member{
					Kind:      skeleton.MemberMethod,
					Name:      content(name, src),
					Signature: sig,
					StartLine: start,
					EndLine:   end,
				})
			}
		case "method_declaration", "constructor_declaration":
			if javaIsPrivate(child, src) {
				continue
			}
			if m := javaExtractMethod(child, src); m != nil {
				members = append(members, *m)
			}
		}
	}
	return members
}

// javaExtractFields extracts each variable_declarator of a field declaration.
// A single declaration can declare multiple fields: int x, y;
func javaExtractFields(node *sitter.Node, src []byte) []skeleton.Member {
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return nil
	}
	typeSig := content(typeNode, src)

	prefix := ""
	if modifiers := findChildByType(node, "modifiers"); modifiers != nil {
		prefix = javaKeywordModifiers(modifiers, src)
	}

	var members []skeleton.Member
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() != "variable_declarator" {
			continue
		}
		name := child.ChildByFieldName("name")
		if name == nil {
			continue
		}
		n := content(name, src)
		sig := strings.TrimSpace(prefix + typeSig + " " + n)
		start, end := nodeLines(child)
		members = append(members, skeleton.Member{
			Kind:      skeleton.MemberField,
			Name:      n,
			Signature: sig,
			StartLine: start,
			EndLine:   end,
		})
	}
	return members
}

// javaExtractMethod extracts a method or constructor signature without the body.
func javaExtractMethod(node *sitter.Node, src []byte) *skeleton.Member {
	name := node.ChildByFieldName("name")
	if name == nil {
		return nil
	}
	sig := strings.TrimSpace(content(node, src))
	if body := node.ChildByFieldName("body"); body != nil {
		sig = strings.TrimSpace(string(src[node.StartByte():body.StartByte()]))
	}
	sig = strings.TrimSuffix(sig, ";")
	sig = javaStripAnnotations(sig)
	start, end := nodeLines(node)
	return &skeleton.Member{
		Kind:      skeleton.MemberMethod,
		Name:      content(name, src),
		Signature: strings.TrimSpace(sig),
		StartLine: start,
		EndLine:   end,
	}
}

// javaKeywordModifiers returns keyword modifiers (static, final, ...) as a
// "static final " style prefix, skipping annotations.
func javaKeywordModifiers(modifiers *sitter.Node, src []byte) string {
	var parts []string
	for i := 0; i < int(modifiers.ChildCount()); i++ {
		child := modifiers.Child(i)
		switch child.Type() {
		case "marker_annotation", "annotation":
			// skip annotations in signatures
		default:
			parts = append(parts, content(child, src))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ") + " "
}

// javaStripAnnotations removes leading annotation lines from a signature.
func javaStripAnnotations(sig string) string {
	lines := strings.Split(sig, "\n")
	start := 0
	for start < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[start]), "@") {
		start++
	}
	if start == 0 {
		return sig
	}
	return strings.Join(lines[start:], "\n")
}
