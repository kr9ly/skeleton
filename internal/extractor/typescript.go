package extractor

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/kr9ly/skeleton/skeleton"
)

type TypeScriptExtractor struct{}

func NewTypeScript() *TypeScriptExtractor {
	return &TypeScriptExtractor{}
}

func (e *TypeScriptExtractor) Extract(src []byte) (*skeleton.File, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(typescript.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	root := tree.RootNode()
	file := &skeleton.File{}
	seen := make(map[string]bool)

	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		switch child.Type() {
		case "import_statement":
			if imp := extractImport(child, src); imp != "" && !seen[imp] {
				seen[imp] = true
				file.Imports = append(file.Imports, imp)
			}
		case "export_statement":
			exports := extractExport(child, src)
			file.Exports = append(file.Exports, exports...)
		}
	}

	return file, nil
}

func extractImport(node *sitter.Node, src []byte) string {
	sourceNode := node.ChildByFieldName("source")
	if sourceNode == nil {
		return ""
	}
	return unquote(sourceNode.Content(src))
}

func extractExport(node *sitter.Node, src []byte) []skeleton.Export {
	// declaration field: export function/class/type/interface/const
	if decl := node.ChildByFieldName("declaration"); decl != nil {
		return extractDeclaration(decl, src, hasDefault(node, src))
	}

	// export clause: export { x, y } or export { x } from "./foo"
	// re-export: export * from "./foo"
	// export default <expr>
	if val := node.ChildByFieldName("value"); val != nil {
		return extractDefaultValue(val, src)
	}

	// export { ... } / export * from "..."
	return extractReExport(node, src)
}

func hasDefault(exportNode *sitter.Node, src []byte) bool {
	for i := 0; i < int(exportNode.ChildCount()); i++ {
		child := exportNode.Child(i)
		if child.Type() == "default" || content(child, src) == "default" {
			return true
		}
	}
	return false
}

func extractDeclaration(decl *sitter.Node, src []byte, isDefault bool) []skeleton.Export {
	prefix := ""
	if isDefault {
		prefix = "default "
	}

	switch decl.Type() {
	case "function_declaration", "function_signature":
		sig := signatureWithoutBody(decl, src)
		name := fieldContent(decl, "name", src)
		return []skeleton.Export{{
			Kind:      skeleton.ExportFunction,
			Name:      name,
			Signature: prefix + sig,
		}}

	case "class_declaration":
		sig := classSignature(decl, src)
		name := fieldContent(decl, "name", src)
		members := extractClassMembers(decl, src)
		return []skeleton.Export{{
			Kind:      skeleton.ExportClass,
			Name:      name,
			Signature: prefix + sig,
			Members:   members,
		}}

	case "interface_declaration":
		name := fieldContent(decl, "name", src)
		members := extractInterfaceMembers(decl, src)
		// メンバーリストがあればヘッダーだけ、なければ全文
		var sig string
		if len(members) > 0 {
			sig = interfaceHeader(decl, src)
		} else {
			sig = typeBodySignature(decl, "interface", name, src)
		}
		return []skeleton.Export{{
			Kind:      skeleton.ExportInterface,
			Name:      name,
			Signature: prefix + sig,
			Members:   members,
		}}

	case "type_alias_declaration":
		name := fieldContent(decl, "name", src)
		sig := strings.TrimSpace(content(decl, src))
		return []skeleton.Export{{
			Kind:      skeleton.ExportType,
			Name:      name,
			Signature: prefix + sig,
		}}

	case "lexical_declaration":
		return extractLexicalDeclaration(decl, src, prefix)

	case "enum_declaration":
		name := fieldContent(decl, "name", src)
		return []skeleton.Export{{
			Kind:      skeleton.ExportType,
			Name:      name,
			Signature: prefix + "enum " + name,
		}}

	default:
		return nil
	}
}

func extractLexicalDeclaration(decl *sitter.Node, src []byte, prefix string) []skeleton.Export {
	keyword := "const"
	if first := decl.Child(0); first != nil {
		keyword = content(first, src)
	}

	var exports []skeleton.Export
	for i := 0; i < int(decl.NamedChildCount()); i++ {
		vd := decl.NamedChild(i)
		if vd.Type() != "variable_declarator" {
			continue
		}

		name := fieldContent(vd, "name", src)
		typeNode := vd.ChildByFieldName("type")
		valueNode := vd.ChildByFieldName("value")

		var sig string
		switch {
		case valueNode != nil && valueNode.Type() == "arrow_function":
			sig = arrowFunctionSignature(keyword, name, typeNode, valueNode, src)
		case typeNode != nil:
			sig = keyword + " " + name + content(typeNode, src)
		default:
			sig = keyword + " " + name
		}

		exports = append(exports, skeleton.Export{
			Kind:      skeleton.ExportVariable,
			Name:      name,
			Signature: prefix + sig,
		})
	}
	return exports
}

func arrowFunctionSignature(keyword, name string, typeNode, valueNode *sitter.Node, src []byte) string {
	if typeNode != nil {
		return keyword + " " + name + content(typeNode, src)
	}
	// type annotation がない場合、arrow function のパラメータと戻り値型を抽出
	sig := signatureWithoutBody(valueNode, src)
	return keyword + " " + name + " = " + sig
}

func signatureWithoutBody(node *sitter.Node, src []byte) string {
	body := node.ChildByFieldName("body")
	if body == nil {
		return strings.TrimSpace(content(node, src))
	}
	start := node.StartByte()
	end := body.StartByte()
	sig := strings.TrimSpace(string(src[start:end]))
	// arrow function の末尾 "=>" を除去
	sig = strings.TrimSpace(strings.TrimSuffix(sig, "=>"))
	return sig
}

func interfaceHeader(node *sitter.Node, src []byte) string {
	body := node.ChildByFieldName("body")
	if body == nil {
		return strings.TrimSpace(content(node, src))
	}
	return strings.TrimSpace(string(src[node.StartByte():body.StartByte()]))
}

func classSignature(node *sitter.Node, src []byte) string {
	body := node.ChildByFieldName("body")
	if body == nil {
		return strings.TrimSpace(content(node, src))
	}
	start := node.StartByte()
	end := body.StartByte()
	return strings.TrimSpace(string(src[start:end]))
}

func typeBodySignature(node *sitter.Node, keyword, name string, src []byte) string {
	body := node.ChildByFieldName("body")
	if body == nil {
		return strings.TrimSpace(content(node, src))
	}

	// フィールド数をカウント
	fieldCount := 0
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		if child.Type() == "property_signature" ||
			child.Type() == "method_signature" ||
			child.Type() == "index_signature" ||
			child.Type() == "call_signature" ||
			child.Type() == "construct_signature" {
			fieldCount++
		}
	}

	const maxInlineFields = 5
	if fieldCount > maxInlineFields {
		// ヘッダー部分（body の前まで）+ フィールド数
		header := strings.TrimSpace(string(src[node.StartByte():body.StartByte()]))
		return header + " { /* " + itoa(fieldCount) + " fields */ }"
	}

	return strings.TrimSpace(content(node, src))
}

func extractDefaultValue(val *sitter.Node, src []byte) []skeleton.Export {
	switch val.Type() {
	case "arrow_function", "function":
		sig := signatureWithoutBody(val, src)
		return []skeleton.Export{{
			Kind:      skeleton.ExportFunction,
			Name:      "default",
			Signature: "default " + sig,
		}}
	default:
		return []skeleton.Export{{
			Kind:      skeleton.ExportVariable,
			Name:      "default",
			Signature: "default " + strings.TrimSpace(content(val, src)),
		}}
	}
}

func extractReExport(node *sitter.Node, src []byte) []skeleton.Export {
	// 全体をそのまま出す（re-export は構造よりもソースが重要）
	text := strings.TrimSpace(content(node, src))
	// "export " prefix を除去して import と同じ扱い
	return []skeleton.Export{{
		Kind:      skeleton.ExportVariable,
		Name:      "",
		Signature: strings.TrimPrefix(text, "export "),
	}}
}

func extractClassMembers(node *sitter.Node, src []byte) []skeleton.Member {
	body := node.ChildByFieldName("body")
	if body == nil {
		return nil
	}

	var members []skeleton.Member
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		switch child.Type() {
		case "method_definition":
			name := fieldContent(child, "name", src)
			kind := skeleton.MemberMethod
			// getter/setter の判定
			for j := 0; j < int(child.ChildCount()); j++ {
				c := child.Child(j)
				text := content(c, src)
				if text == "get" && c.Type() != "identifier" {
					kind = skeleton.MemberGetter
				} else if text == "set" && c.Type() != "identifier" {
					kind = skeleton.MemberSetter
				}
			}
			sig := signatureWithoutBody(child, src)
			members = append(members, skeleton.Member{Kind: kind, Name: name, Signature: sig})

		case "public_field_definition":
			name := fieldContent(child, "name", src)
			typeNode := child.ChildByFieldName("type")
			sig := name
			if typeNode != nil {
				sig = name + content(typeNode, src)
			}
			members = append(members, skeleton.Member{Kind: skeleton.MemberField, Name: name, Signature: sig})
		}
	}
	return members
}

func extractInterfaceMembers(node *sitter.Node, src []byte) []skeleton.Member {
	body := node.ChildByFieldName("body")
	if body == nil {
		return nil
	}

	var members []skeleton.Member
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		switch child.Type() {
		case "method_signature":
			name := fieldContent(child, "name", src)
			sig := strings.TrimSpace(content(child, src))
			// 末尾のセミコロンを除去
			sig = strings.TrimRight(sig, ";")
			members = append(members, skeleton.Member{Kind: skeleton.MemberMethod, Name: name, Signature: sig})

		case "property_signature":
			name := fieldContent(child, "name", src)
			sig := strings.TrimSpace(content(child, src))
			sig = strings.TrimRight(sig, ";")
			members = append(members, skeleton.Member{Kind: skeleton.MemberField, Name: name, Signature: sig})
		}
	}
	return members
}

func content(node *sitter.Node, src []byte) string {
	return node.Content(src)
}

func fieldContent(node *sitter.Node, field string, src []byte) string {
	child := node.ChildByFieldName(field)
	if child == nil {
		return ""
	}
	return child.Content(src)
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
