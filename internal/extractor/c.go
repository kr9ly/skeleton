package extractor

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/c"

	"github.com/kr9ly/skeleton/skeleton"
)

type CExtractor struct{}

func NewC() *CExtractor {
	return &CExtractor{}
}

func (e *CExtractor) Extract(src []byte) (*skeleton.File, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(c.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	root := tree.RootNode()
	file := &skeleton.File{}
	seen := make(map[string]bool)

	// 関数定義の名前を集めて、宣言（プロトタイプ）との重複を排除
	definedFuncs := make(map[string]bool)
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if child.Type() == "function_definition" {
			if cNodeHasStorageClass(child, src, "static") {
				continue
			}
			if name := cFuncName(child, src); name != "" {
				definedFuncs[name] = true
			}
		}
	}

	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		switch child.Type() {
		case "preproc_include":
			if imp := cExtractInclude(child, src); imp != "" {
				if !seen[imp] {
					seen[imp] = true
					file.Imports = append(file.Imports, imp)
				}
			}

		case "preproc_def":
			if exp := cExtractDefine(child, src); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}

		case "preproc_function_def":
			if exp := cExtractFunctionMacro(child, src); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}

		case "type_definition":
			if exp := cExtractTypedef(child, src); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}

		case "enum_specifier":
			if exp := cExtractEnum(child, src); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}

		case "struct_specifier":
			if exp := cExtractStruct(child, src); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}

		case "declaration":
			if cNodeHasStorageClass(child, src, "static") {
				continue
			}
			exports := cExtractDeclaration(child, src, definedFuncs)
			file.Exports = append(file.Exports, exports...)

		case "function_definition":
			if cNodeHasStorageClass(child, src, "static") {
				continue
			}
			if exp := cExtractFuncDef(child, src); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}
		}
	}

	return file, nil
}

func cExtractInclude(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "system_lib_string":
			// <stdio.h> → stdio.h
			s := content(child, src)
			if len(s) >= 2 {
				return s[1 : len(s)-1]
			}
			return s
		case "string_literal":
			return unquote(content(child, src))
		}
	}
	return ""
}

func cExtractDefine(node *sitter.Node, src []byte) *skeleton.Export {
	name := ""
	value := ""
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "identifier":
			name = content(child, src)
		case "preproc_arg":
			value = strings.TrimSpace(content(child, src))
		}
	}
	if name == "" {
		return nil
	}
	sig := "#define " + name
	if value != "" {
		sig += " " + value
	}
	return &skeleton.Export{
		Kind:      skeleton.ExportVariable,
		Name:      name,
		Signature: sig,
	}
}

func cExtractFunctionMacro(node *sitter.Node, src []byte) *skeleton.Export {
	name := ""
	params := ""
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "identifier":
			name = content(child, src)
		case "preproc_params":
			params = content(child, src)
		}
	}
	if name == "" {
		return nil
	}
	return &skeleton.Export{
		Kind:      skeleton.ExportFunction,
		Name:      name,
		Signature: "#define " + name + params,
	}
}

func cExtractTypedef(node *sitter.Node, src []byte) *skeleton.Export {
	text := strings.TrimSuffix(strings.TrimSpace(content(node, src)), ";")

	// typedef で定義される名前: 最後の type_identifier
	// ただし関数ポインタ typedef の場合は function_declarator 内を探す
	name := ""
	for i := int(node.NamedChildCount()) - 1; i >= 0; i-- {
		child := node.NamedChild(i)
		if child.Type() == "type_identifier" {
			name = content(child, src)
			break
		}
		if child.Type() == "function_declarator" {
			name = cFindTypeIdentifierIn(child, src)
			break
		}
	}
	if name == "" {
		return nil
	}

	return &skeleton.Export{
		Kind:      skeleton.ExportType,
		Name:      name,
		Signature: text,
	}
}

func cFindTypeIdentifierIn(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "type_identifier" {
			return content(child, src)
		}
		if child.Type() == "parenthesized_declarator" || child.Type() == "pointer_declarator" {
			if name := cFindTypeIdentifierIn(child, src); name != "" {
				return name
			}
		}
	}
	return ""
}

func cExtractEnum(node *sitter.Node, src []byte) *skeleton.Export {
	name := ""
	var members []skeleton.Member

	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "type_identifier":
			name = content(child, src)
		case "enumerator_list":
			members = cExtractEnumerators(child, src)
		}
	}

	if name == "" {
		return nil
	}

	return &skeleton.Export{
		Kind:      skeleton.ExportType,
		Name:      name,
		Signature: "enum " + name,
		Members:   members,
	}
}

func cExtractEnumerators(node *sitter.Node, src []byte) []skeleton.Member {
	var members []skeleton.Member
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() != "enumerator" {
			continue
		}
		eName := ""
		for j := 0; j < int(child.NamedChildCount()); j++ {
			gc := child.NamedChild(j)
			if gc.Type() == "identifier" {
				eName = content(gc, src)
				break
			}
		}
		if eName != "" {
			members = append(members, skeleton.Member{
				Kind:      skeleton.MemberField,
				Name:      eName,
				Signature: eName,
			})
		}
	}
	return members
}

func cExtractStruct(node *sitter.Node, src []byte) *skeleton.Export {
	name := ""
	var members []skeleton.Member

	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "type_identifier":
			name = content(child, src)
		case "field_declaration_list":
			members = cExtractStructFields(child, src)
		}
	}

	if name == "" {
		return nil
	}

	return &skeleton.Export{
		Kind:      skeleton.ExportClass,
		Name:      name,
		Signature: "struct " + name,
		Members:   members,
	}
}

func cExtractStructFields(node *sitter.Node, src []byte) []skeleton.Member {
	var members []skeleton.Member
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() != "field_declaration" {
			continue
		}
		fieldName := cFindFieldIdentifier(child, src)
		sig := strings.TrimSuffix(strings.TrimSpace(content(child, src)), ";")
		if fieldName == "" {
			fieldName = sig
		}
		members = append(members, skeleton.Member{
			Kind:      skeleton.MemberField,
			Name:      fieldName,
			Signature: sig,
		})
	}
	return members
}

func cFindFieldIdentifier(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "field_identifier" {
			return content(child, src)
		}
		if child.Type() == "pointer_declarator" || child.Type() == "array_declarator" {
			if name := cFindFieldIdentifier(child, src); name != "" {
				return name
			}
		}
	}
	return ""
}

// cExtractDeclaration は declaration ノードから関数プロトタイプ or 変数宣言を抽出する。
func cExtractDeclaration(node *sitter.Node, src []byte, definedFuncs map[string]bool) []skeleton.Export {
	// 関数プロトタイプか変数宣言かを判別
	if fd := cFindFuncDeclarator(node); fd != nil {
		name := cFuncDeclaratorName(fd, src)
		if name == "" || definedFuncs[name] {
			return nil // 定義がある場合はスキップ
		}
		sig := strings.TrimSuffix(strings.TrimSpace(content(node, src)), ";")
		return []skeleton.Export{{
			Kind:      skeleton.ExportFunction,
			Name:      name,
			Signature: sig,
		}}
	}

	// 変数宣言
	var exports []skeleton.Export
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "init_declarator" {
			name := cInitDeclaratorName(child, src)
			if name != "" {
				sig := strings.TrimSuffix(strings.TrimSpace(content(node, src)), ";")
				exports = append(exports, skeleton.Export{
					Kind:      skeleton.ExportVariable,
					Name:      name,
					Signature: sig,
				})
			}
		} else if child.Type() == "identifier" {
			// int x; のような単純宣言
			name := content(child, src)
			sig := strings.TrimSuffix(strings.TrimSpace(content(node, src)), ";")
			exports = append(exports, skeleton.Export{
				Kind:      skeleton.ExportVariable,
				Name:      name,
				Signature: sig,
			})
		}
	}
	return exports
}

func cExtractFuncDef(node *sitter.Node, src []byte) *skeleton.Export {
	name := cFuncName(node, src)
	if name == "" {
		return nil
	}

	// body (compound_statement) の手前までがシグネチャ
	body := cFindChildByType(node, "compound_statement")
	var sig string
	if body != nil {
		sig = strings.TrimSpace(string(src[node.StartByte():body.StartByte()]))
	} else {
		sig = strings.TrimSpace(content(node, src))
	}

	return &skeleton.Export{
		Kind:      skeleton.ExportFunction,
		Name:      name,
		Signature: sig,
	}
}

func cNodeHasStorageClass(node *sitter.Node, src []byte, class string) bool {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "storage_class_specifier" && content(child, src) == class {
			return true
		}
	}
	return false
}

func cFuncName(node *sitter.Node, src []byte) string {
	fd := cFindFuncDeclarator(node)
	if fd == nil {
		return ""
	}
	return cFuncDeclaratorName(fd, src)
}

func cFindFuncDeclarator(node *sitter.Node) *sitter.Node {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "function_declarator" {
			return child
		}
		// pointer_declarator の中に function_declarator がある場合（戻り値がポインタ）
		if child.Type() == "pointer_declarator" {
			if fd := cFindFuncDeclarator(child); fd != nil {
				return fd
			}
		}
	}
	return nil
}

func cFuncDeclaratorName(fd *sitter.Node, src []byte) string {
	for i := 0; i < int(fd.NamedChildCount()); i++ {
		child := fd.NamedChild(i)
		if child.Type() == "identifier" {
			return content(child, src)
		}
	}
	return ""
}

func cInitDeclaratorName(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "identifier" {
			return content(child, src)
		}
		if child.Type() == "pointer_declarator" {
			return cInitDeclaratorName(child, src)
		}
	}
	return ""
}

func cFindChildByType(node *sitter.Node, typeName string) *sitter.Node {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == typeName {
			return child
		}
	}
	return nil
}
