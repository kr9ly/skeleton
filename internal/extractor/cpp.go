package extractor

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/cpp"

	"github.com/kr9ly/skeleton/skeleton"
)

type CPPExtractor struct{}

func NewCPP() *CPPExtractor {
	return &CPPExtractor{}
}

func (e *CPPExtractor) Extract(src []byte) (*skeleton.File, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(cpp.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	root := tree.RootNode()
	file := &skeleton.File{}
	seen := make(map[string]bool)

	cppExtractNodes(root, src, "", file, seen)

	return file, nil
}

// cppExtractNodes は translation_unit または namespace の declaration_list を走査する。
// prefix は "ns::" の形式（空文字列 = トップレベル）。
func cppExtractNodes(node *sitter.Node, src []byte, prefix string, file *skeleton.File, seen map[string]bool) {
	// 関数定義の名前を先に収集して、プロトタイプとの重複を排除する
	definedFuncs := make(map[string]bool)
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "function_definition" {
			if cNodeHasStorageClass(child, src, "static") {
				continue
			}
			if name := cFuncName(child, src); name != "" {
				definedFuncs[prefix+name] = true
			}
		}
		// template で包まれた function_definition も収集
		if child.Type() == "template_declaration" {
			inner := cppTemplateInner(child)
			if inner != nil && inner.Type() == "function_definition" {
				if name := cFuncName(inner, src); name != "" {
					definedFuncs[prefix+name] = true
				}
			}
		}
	}

	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
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

		case "alias_declaration":
			if exp := cppExtractAlias(child, src, prefix); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}

		case "type_definition":
			if exp := cExtractTypedef(child, src); exp != nil {
				exp.Name = prefix + exp.Name
				file.Exports = append(file.Exports, *exp)
			}

		case "enum_specifier":
			if exp := cppExtractEnum(child, src, prefix); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}

		case "struct_specifier":
			if exp := cExtractStruct(child, src); exp != nil {
				exp.Name = prefix + exp.Name
				exp.Signature = "struct " + exp.Name
				file.Exports = append(file.Exports, *exp)
			}

		case "class_specifier":
			if exp := cppExtractClass(child, src, prefix); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}

		case "namespace_definition":
			cppExtractNamespace(child, src, prefix, file, seen)

		case "template_declaration":
			exports := cppExtractTemplate(child, src, prefix, definedFuncs)
			file.Exports = append(file.Exports, exports...)

		case "declaration":
			if cNodeHasStorageClass(child, src, "static") {
				continue
			}
			// qualified_identifier を持つ定義（クラス外メソッド定義）はスキップ
			if cppIsOutOfLineDefinition(child, src) {
				continue
			}
			exports := cppExtractDeclaration(child, src, prefix, definedFuncs)
			file.Exports = append(file.Exports, exports...)

		case "function_definition":
			if cNodeHasStorageClass(child, src, "static") {
				continue
			}
			// クラス外のメソッド定義（Foo::bar）はスキップ
			if cppFuncIsOutOfLine(child, src) {
				continue
			}
			if exp := cppExtractFuncDef(child, src, prefix); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}
		}
	}
}

// cppExtractNamespace は namespace_definition を再帰的に処理する。
func cppExtractNamespace(node *sitter.Node, src []byte, prefix string, file *skeleton.File, seen map[string]bool) {
	nsName := ""
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "namespace_identifier" || child.Type() == "identifier" {
			nsName = content(child, src)
			break
		}
	}

	newPrefix := prefix
	if nsName != "" {
		newPrefix = prefix + nsName + "::"
	}

	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "declaration_list" {
			cppExtractNodes(child, src, newPrefix, file, seen)
			break
		}
	}
}

// cppExtractClass は class_specifier を処理して public メンバーを抽出する。
func cppExtractClass(node *sitter.Node, src []byte, prefix string) *skeleton.Export {
	className := ""
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "type_identifier" {
			className = content(child, src)
			break
		}
	}
	if className == "" {
		return nil
	}

	var members []skeleton.Member

	// field_declaration_list を探す
	fdl := cFindChildByType(node, "field_declaration_list")
	if fdl != nil {
		// class のデフォルトは private
		currentAccess := "private"
		for i := 0; i < int(fdl.NamedChildCount()); i++ {
			child := fdl.NamedChild(i)
			switch child.Type() {
			case "access_specifier":
				spec := strings.TrimSuffix(content(child, src), ":")
				currentAccess = strings.TrimSpace(spec)
			case "declaration":
				if currentAccess != "public" {
					continue
				}
				if m := cppExtractMemberDecl(child, src, className); m != nil {
					members = append(members, *m)
				}
			case "field_declaration":
				if currentAccess != "public" {
					continue
				}
				if m := cppExtractMemberField(child, src); m != nil {
					members = append(members, *m)
				}
			case "function_definition":
				// inline 定義されたメソッド
				if currentAccess != "public" {
					continue
				}
				if m := cppExtractMemberFuncDef(child, src); m != nil {
					members = append(members, *m)
				}
			}
		}
	}

	fullName := prefix + className
	return &skeleton.Export{
		Kind:      skeleton.ExportClass,
		Name:      fullName,
		Signature: "class " + fullName,
		Members:   members,
	}
}

// cppExtractMemberDecl は field_declaration_list 内の declaration ノードを処理する。
// コンストラクタ・デストラクタ宣言がここに来る。
func cppExtractMemberDecl(node *sitter.Node, src []byte, className string) *skeleton.Member {
	sig := strings.TrimSuffix(strings.TrimSpace(content(node, src)), ";")

	// function_declarator を持つ → メソッド宣言
	if fd := cFindFuncDeclarator(node); fd != nil {
		name := cppMemberDeclName(node, src)
		if name == "" {
			name = sig
		}
		return &skeleton.Member{
			Kind:      skeleton.MemberMethod,
			Name:      name,
			Signature: sig,
		}
	}

	// デストラクタ: destructor_name ノード or "~" 始まりの identifier
	if name := cppFindDestructorName(node, src); name != "" {
		return &skeleton.Member{
			Kind:      skeleton.MemberMethod,
			Name:      name,
			Signature: sig,
		}
	}

	// フィールド宣言
	fieldName := cFindFieldIdentifier(node, src)
	if fieldName == "" {
		// identifier も試す
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child.Type() == "identifier" {
				fieldName = content(child, src)
				break
			}
		}
	}
	if fieldName == "" {
		fieldName = sig
	}
	return &skeleton.Member{
		Kind:      skeleton.MemberField,
		Name:      fieldName,
		Signature: sig,
	}
}

// cppMemberDeclName は declaration ノードから関数名を取得する。
// コンストラクタ（ClassName()）も含む。
func cppMemberDeclName(node *sitter.Node, src []byte) string {
	fd := cFindFuncDeclarator(node)
	if fd == nil {
		return ""
	}
	return cFuncDeclaratorName(fd, src)
}

// cppFindDestructorName はデストラクタ名（~ClassName）を探す。
func cppFindDestructorName(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		// tree-sitter C++ パーサーが destructor_name を使う場合
		if child.Type() == "destructor_name" {
			return content(child, src)
		}
		// function_declarator の中を探す
		if child.Type() == "function_declarator" {
			for j := 0; j < int(child.NamedChildCount()); j++ {
				gc := child.NamedChild(j)
				if gc.Type() == "destructor_name" {
					return content(gc, src)
				}
				// identifier が "~" で始まる場合
				if gc.Type() == "identifier" {
					name := content(gc, src)
					if strings.HasPrefix(name, "~") {
						return name
					}
				}
			}
		}
	}
	return ""
}

// cppExtractMemberField は field_declaration ノードからメンバーを抽出する。
func cppExtractMemberField(node *sitter.Node, src []byte) *skeleton.Member {
	sig := strings.TrimSuffix(strings.TrimSpace(content(node, src)), ";")

	// function_declarator を持つ → メソッド宣言（field_declaration に来ることもある）
	if fd := cFindFuncDeclarator(node); fd != nil {
		name := cFuncDeclaratorName(fd, src)
		if name == "" {
			name = sig
		}
		return &skeleton.Member{
			Kind:      skeleton.MemberMethod,
			Name:      name,
			Signature: sig,
		}
	}

	// フィールド
	fieldName := cFindFieldIdentifier(node, src)
	if fieldName == "" {
		fieldName = sig
	}
	return &skeleton.Member{
		Kind:      skeleton.MemberField,
		Name:      fieldName,
		Signature: sig,
	}
}

// cppExtractMemberFuncDef は inline 定義されたメソッドを処理する。
func cppExtractMemberFuncDef(node *sitter.Node, src []byte) *skeleton.Member {
	name := cFuncName(node, src)
	if name == "" {
		// destructor
		name = cppFindDestructorName(node, src)
	}
	if name == "" {
		return nil
	}

	body := cFindChildByType(node, "compound_statement")
	var sig string
	if body != nil {
		sig = strings.TrimSpace(string(src[node.StartByte():body.StartByte()]))
	} else {
		sig = strings.TrimSpace(content(node, src))
	}

	return &skeleton.Member{
		Kind:      skeleton.MemberMethod,
		Name:      name,
		Signature: sig,
	}
}

// cppExtractAlias は alias_declaration（using Name = Type;）を処理する。
func cppExtractAlias(node *sitter.Node, src []byte, prefix string) *skeleton.Export {
	name := ""
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "type_identifier" {
			name = content(child, src)
			break
		}
	}
	if name == "" {
		return nil
	}

	sig := strings.TrimSuffix(strings.TrimSpace(content(node, src)), ";")
	fullName := prefix + name
	return &skeleton.Export{
		Kind:      skeleton.ExportType,
		Name:      fullName,
		Signature: sig,
	}
}

// cppExtractEnum は enum_specifier を処理する。enum class も対応。
func cppExtractEnum(node *sitter.Node, src []byte, prefix string) *skeleton.Export {
	name := ""
	isClass := false
	var members []skeleton.Member

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "class", "struct":
			isClass = true
		}
	}
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

	keyword := "enum"
	if isClass {
		keyword = "enum class"
	}

	fullName := prefix + name
	return &skeleton.Export{
		Kind:      skeleton.ExportType,
		Name:      fullName,
		Signature: keyword + " " + fullName,
		Members:   members,
	}
}

// cppExtractTemplate は template_declaration を処理する。
func cppExtractTemplate(node *sitter.Node, src []byte, prefix string, definedFuncs map[string]bool) []skeleton.Export {
	// template_parameter_list を取得
	templateParams := ""
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "template_parameter_list" {
			templateParams = content(child, src)
			break
		}
	}

	inner := cppTemplateInner(node)
	if inner == nil {
		return nil
	}

	switch inner.Type() {
	case "function_definition":
		if cNodeHasStorageClass(inner, src, "static") {
			return nil
		}
		name := cFuncName(inner, src)
		if name == "" {
			return nil
		}
		body := cFindChildByType(inner, "compound_statement")
		var sig string
		if body != nil {
			sig = strings.TrimSpace(string(src[inner.StartByte():body.StartByte()]))
		} else {
			sig = strings.TrimSpace(content(inner, src))
		}
		fullName := prefix + name
		return []skeleton.Export{{
			Kind:      skeleton.ExportFunction,
			Name:      fullName,
			Signature: "template" + templateParams + " " + sig,
		}}

	case "declaration":
		if cNodeHasStorageClass(inner, src, "static") {
			return nil
		}
		exports := cppExtractDeclaration(inner, src, prefix, definedFuncs)
		for i := range exports {
			exports[i].Signature = "template" + templateParams + " " + exports[i].Signature
		}
		return exports

	case "class_specifier":
		exp := cppExtractClass(inner, src, prefix)
		if exp != nil {
			exp.Signature = "template" + templateParams + " " + exp.Signature
		}
		if exp != nil {
			return []skeleton.Export{*exp}
		}
	}

	return nil
}

// cppTemplateInner は template_declaration の本体（function/class/declaration）を返す。
func cppTemplateInner(node *sitter.Node) *sitter.Node {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "function_definition", "declaration", "class_specifier", "struct_specifier":
			return child
		}
	}
	return nil
}

// cppExtractDeclaration は declaration ノード（関数プロトタイプまたは変数宣言）を処理する。
func cppExtractDeclaration(node *sitter.Node, src []byte, prefix string, definedFuncs map[string]bool) []skeleton.Export {
	// 関数プロトタイプ
	if fd := cFindFuncDeclarator(node); fd != nil {
		name := cFuncDeclaratorName(fd, src)
		fullName := prefix + name
		if name == "" || definedFuncs[fullName] {
			return nil
		}
		sig := strings.TrimSuffix(strings.TrimSpace(content(node, src)), ";")
		return []skeleton.Export{{
			Kind:      skeleton.ExportFunction,
			Name:      fullName,
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
				fullName := prefix + name
				sig := strings.TrimSuffix(strings.TrimSpace(content(node, src)), ";")
				exports = append(exports, skeleton.Export{
					Kind:      skeleton.ExportVariable,
					Name:      fullName,
					Signature: sig,
				})
			}
		} else if child.Type() == "identifier" {
			name := content(child, src)
			fullName := prefix + name
			sig := strings.TrimSuffix(strings.TrimSpace(content(node, src)), ";")
			exports = append(exports, skeleton.Export{
				Kind:      skeleton.ExportVariable,
				Name:      fullName,
				Signature: sig,
			})
		}
	}
	return exports
}

// cppExtractFuncDef はトップレベルの function_definition を処理する。
func cppExtractFuncDef(node *sitter.Node, src []byte, prefix string) *skeleton.Export {
	name := cFuncName(node, src)
	if name == "" {
		return nil
	}

	body := cFindChildByType(node, "compound_statement")
	var sig string
	if body != nil {
		sig = strings.TrimSpace(string(src[node.StartByte():body.StartByte()]))
	} else {
		sig = strings.TrimSpace(content(node, src))
	}

	fullName := prefix + name
	return &skeleton.Export{
		Kind:      skeleton.ExportFunction,
		Name:      fullName,
		Signature: sig,
	}
}

// cppIsOutOfLineDefinition は declaration が Foo::bar 形式の定義かを判定する。
func cppIsOutOfLineDefinition(node *sitter.Node, src []byte) bool {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "function_declarator" {
			// function_declarator 直下に qualified_identifier があれば out-of-line
			for j := 0; j < int(child.NamedChildCount()); j++ {
				gc := child.NamedChild(j)
				if gc.Type() == "qualified_identifier" {
					return true
				}
			}
		}
		if child.Type() == "pointer_declarator" {
			if inner := cFindFuncDeclarator(child); inner != nil {
				for j := 0; j < int(inner.NamedChildCount()); j++ {
					gc := inner.NamedChild(j)
					if gc.Type() == "qualified_identifier" {
						return true
					}
				}
			}
		}
	}
	return false
}

// cppFuncIsOutOfLine は function_definition が Foo::bar 形式（クラス外定義）かを判定する。
func cppFuncIsOutOfLine(node *sitter.Node, src []byte) bool {
	fd := cFindFuncDeclarator(node)
	if fd == nil {
		return false
	}
	for i := 0; i < int(fd.NamedChildCount()); i++ {
		child := fd.NamedChild(i)
		if child.Type() == "qualified_identifier" {
			return true
		}
	}
	return false
}
