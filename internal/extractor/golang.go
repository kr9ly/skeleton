package extractor

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"

	"github.com/kr9ly/skeleton/skeleton"
)

type GoExtractor struct{}

func NewGo() *GoExtractor {
	return &GoExtractor{}
}

func (e *GoExtractor) Extract(src []byte) (*skeleton.File, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(golang.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	root := tree.RootNode()
	file := &skeleton.File{}
	seen := make(map[string]bool)

	// メソッドをレシーバー型ごとに集める
	methods := make(map[string][]skeleton.Member)

	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		switch child.Type() {
		case "import_declaration":
			for _, imp := range goExtractImports(child, src) {
				if !seen[imp] {
					seen[imp] = true
					file.Imports = append(file.Imports, imp)
				}
			}
		case "function_declaration":
			if exp := goExtractFunction(child, src); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}
		case "method_declaration":
			recv, member := goExtractMethod(child, src)
			if recv != "" && member != nil {
				methods[recv] = append(methods[recv], *member)
			}
		case "type_declaration":
			exports := goExtractTypeDecl(child, src)
			file.Exports = append(file.Exports, exports...)
		case "var_declaration":
			exports := goExtractVarDecl(child, src)
			file.Exports = append(file.Exports, exports...)
		case "const_declaration":
			exports := goExtractConstDecl(child, src)
			file.Exports = append(file.Exports, exports...)
		}
	}

	// メソッドを対応する型の Members に合流
	for i := range file.Exports {
		exp := &file.Exports[i]
		if exp.Kind == skeleton.ExportClass || exp.Kind == skeleton.ExportInterface {
			if ms, ok := methods[exp.Name]; ok {
				exp.Members = append(exp.Members, ms...)
				delete(methods, exp.Name)
			}
		}
	}

	// 型宣言が見つからなかったメソッドはトップレベル関数として追加
	for typeName, ms := range methods {
		for _, m := range ms {
			file.Exports = append(file.Exports, skeleton.Export{
				Kind:      skeleton.ExportFunction,
				Name:      m.Name,
				Signature: "func (" + typeName + ") " + m.Signature[len("func "):],
			})
		}
	}

	return file, nil
}

func goExtractImports(node *sitter.Node, src []byte) []string {
	var imports []string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "import_spec":
			path := child.ChildByFieldName("path")
			if path != nil {
				imports = append(imports, unquote(content(path, src)))
			}
		case "import_spec_list":
			for j := 0; j < int(child.NamedChildCount()); j++ {
				spec := child.NamedChild(j)
				if spec.Type() == "import_spec" {
					path := spec.ChildByFieldName("path")
					if path != nil {
						imports = append(imports, unquote(content(path, src)))
					}
				}
			}
		}
	}
	return imports
}

func goExtractFunction(node *sitter.Node, src []byte) *skeleton.Export {
	name := fieldContent(node, "name", src)
	if !isExported(name) {
		return nil
	}

	sig := goFuncSignature(node, src)
	return &skeleton.Export{
		Kind:      skeleton.ExportFunction,
		Name:      name,
		Signature: sig,
	}
}

func goExtractMethod(node *sitter.Node, src []byte) (string, *skeleton.Member) {
	name := fieldContent(node, "name", src)
	if !isExported(name) {
		return "", nil
	}

	// レシーバー型名を取得
	recvNode := node.ChildByFieldName("receiver")
	if recvNode == nil {
		return "", nil
	}
	typeName := goReceiverTypeName(recvNode, src)
	if typeName == "" {
		return "", nil
	}

	sig := goFuncSignature(node, src)
	return typeName, &skeleton.Member{
		Kind:      skeleton.MemberMethod,
		Name:      name,
		Signature: sig,
	}
}

func goReceiverTypeName(node *sitter.Node, src []byte) string {
	// parameter_list の中の parameter_declaration を探す
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "parameter_declaration" {
			typeNode := child.ChildByFieldName("type")
			if typeNode == nil {
				continue
			}
			// *Type のポインタレシーバーの場合
			if typeNode.Type() == "pointer_type" {
				for j := 0; j < int(typeNode.NamedChildCount()); j++ {
					inner := typeNode.NamedChild(j)
					if inner.Type() == "type_identifier" {
						return content(inner, src)
					}
				}
			}
			// 値レシーバーの場合
			if typeNode.Type() == "type_identifier" {
				return content(typeNode, src)
			}
		}
	}
	return ""
}

func goFuncSignature(node *sitter.Node, src []byte) string {
	body := node.ChildByFieldName("body")
	if body == nil {
		return strings.TrimSpace(content(node, src))
	}
	sig := strings.TrimSpace(string(src[node.StartByte():body.StartByte()]))
	return sig
}

func goExtractTypeDecl(node *sitter.Node, src []byte) []skeleton.Export {
	var exports []skeleton.Export
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "type_alias" {
			if exp := goExtractTypeAlias(child, src); exp != nil {
				exports = append(exports, *exp)
			}
			continue
		}
		if child.Type() != "type_spec" {
			continue
		}
		name := fieldContent(child, "name", src)
		if !isExported(name) {
			continue
		}
		typeNode := child.ChildByFieldName("type")
		if typeNode == nil {
			continue
		}

		switch typeNode.Type() {
		case "struct_type":
			members := goExtractStructFields(typeNode, src)
			exports = append(exports, skeleton.Export{
				Kind:      skeleton.ExportClass,
				Name:      name,
				Signature: "type " + name + " struct",
				Members:   members,
			})
		case "interface_type":
			members := goExtractInterfaceMethods(typeNode, src)
			exports = append(exports, skeleton.Export{
				Kind:      skeleton.ExportInterface,
				Name:      name,
				Signature: "type " + name + " interface",
				Members:   members,
			})
		default:
			// type alias / named type
			sig := strings.TrimSpace(content(child, src))
			exports = append(exports, skeleton.Export{
				Kind:      skeleton.ExportType,
				Name:      name,
				Signature: "type " + sig,
			})
		}
	}
	return exports
}

func goExtractTypeAlias(node *sitter.Node, src []byte) *skeleton.Export {
	// type_alias: first child is type_identifier (name), rest is the aliased type
	var name string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "type_identifier" {
			name = content(child, src)
			break
		}
	}
	if !isExported(name) {
		return nil
	}
	sig := "type " + strings.TrimSpace(content(node, src))
	return &skeleton.Export{
		Kind:      skeleton.ExportType,
		Name:      name,
		Signature: sig,
	}
}

func goExtractStructFields(node *sitter.Node, src []byte) []skeleton.Member {
	body := node.ChildByFieldName("body")
	if body == nil {
		// struct_type の中の field_declaration_list を探す
		body = findChildByType(node, "field_declaration_list")
	}
	if body == nil {
		return nil
	}

	var members []skeleton.Member
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		if child.Type() != "field_declaration" {
			continue
		}
		name := goStructFieldName(child, src)
		if name == "" {
			// 埋め込みフィールド: field_identifier がない場合
			typeNode := goFieldType(child, src)
			if typeNode != nil {
				typeName := content(typeNode, src)
				if isExported(typeName) || strings.HasPrefix(typeName, "*") {
					members = append(members, skeleton.Member{
						Kind:      skeleton.MemberField,
						Name:      typeName,
						Signature: typeName,
					})
				}
			}
			continue
		}
		if !isExported(name) {
			continue
		}
		typeNode := goFieldType(child, src)
		sig := name
		if typeNode != nil {
			sig = name + " " + content(typeNode, src)
		}
		members = append(members, skeleton.Member{
			Kind:      skeleton.MemberField,
			Name:      name,
			Signature: sig,
		})
	}
	return members
}

func goExtractInterfaceMethods(node *sitter.Node, src []byte) []skeleton.Member {
	var members []skeleton.Member
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "method_spec", "method_elem":
			name := goInterfaceMethodName(child, src)
			if name == "" {
				continue
			}
			sig := strings.TrimSpace(content(child, src))
			members = append(members, skeleton.Member{
				Kind:      skeleton.MemberMethod,
				Name:      name,
				Signature: sig,
			})
		case "type_identifier", "qualified_type":
			// 埋め込みインターフェース
			typeName := content(child, src)
			members = append(members, skeleton.Member{
				Kind:      skeleton.MemberField,
				Name:      typeName,
				Signature: typeName,
			})
		case "type_elem":
			// 埋め込みインターフェース (type_elem > qualified_type|type_identifier)
			typeName := strings.TrimSpace(content(child, src))
			members = append(members, skeleton.Member{
				Kind:      skeleton.MemberField,
				Name:      typeName,
				Signature: typeName,
			})
		}
	}
	return members
}

func goExtractVarDecl(node *sitter.Node, src []byte) []skeleton.Export {
	var exports []skeleton.Export
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() != "var_spec" {
			continue
		}
		name := goSpecName(child, src)
		if !isExported(name) {
			continue
		}
		typeName := goSpecType(child, src)
		sig := "var " + name
		if typeName != "" {
			sig += " " + typeName
		}
		exports = append(exports, skeleton.Export{
			Kind:      skeleton.ExportVariable,
			Name:      name,
			Signature: sig,
		})
	}
	return exports
}

func goExtractConstDecl(node *sitter.Node, src []byte) []skeleton.Export {
	var exports []skeleton.Export
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() != "const_spec" {
			continue
		}
		name := goSpecName(child, src)
		if !isExported(name) {
			continue
		}
		typeName := goSpecType(child, src)
		sig := "const " + name
		if typeName != "" {
			sig += " " + typeName
		}
		exports = append(exports, skeleton.Export{
			Kind:      skeleton.ExportVariable,
			Name:      name,
			Signature: sig,
		})
	}
	return exports
}

// goSpecName は const_spec / var_spec の名前を返す。
func goSpecName(node *sitter.Node, src []byte) string {
	if name := fieldContent(node, "name", src); name != "" {
		return name
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "identifier" {
			return content(child, src)
		}
	}
	return ""
}

// goSpecType は const_spec / var_spec の型名を返す。
func goSpecType(node *sitter.Node, src []byte) string {
	if t := node.ChildByFieldName("type"); t != nil {
		return content(t, src)
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "type_identifier" {
			return content(child, src)
		}
	}
	return ""
}

func isExported(name string) bool {
	if name == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(r)
}

func goStructFieldName(node *sitter.Node, src []byte) string {
	if name := fieldContent(node, "name", src); name != "" {
		return name
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "field_identifier" {
			return content(child, src)
		}
	}
	return ""
}

func goFieldType(node *sitter.Node, src []byte) *sitter.Node {
	if t := node.ChildByFieldName("type"); t != nil {
		return t
	}
	// field_identifier の次のノードが型
	foundName := false
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "field_identifier" {
			foundName = true
			continue
		}
		if foundName {
			return child
		}
	}
	return nil
}

func goInterfaceMethodName(node *sitter.Node, src []byte) string {
	// method_elem uses field_identifier, method_spec uses name field
	if name := fieldContent(node, "name", src); name != "" {
		return name
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "field_identifier" {
			return content(child, src)
		}
	}
	return ""
}

func findChildByType(node *sitter.Node, typeName string) *sitter.Node {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == typeName {
			return child
		}
	}
	return nil
}
