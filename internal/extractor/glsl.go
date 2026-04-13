package extractor

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/kr9ly/skeleton/internal/treesitter/glsl"
	"github.com/kr9ly/skeleton/skeleton"
)

// GLSLExtractor は GLSL (.glsl/.vert/.frag/.comp 等) ファイルを解析する。
// tree-sitter-glsl は C ベースの文法なので C ヘルパーを再利用する。
type GLSLExtractor struct{}

func NewGLSL() *GLSLExtractor {
	return &GLSLExtractor{}
}

// GLSL の storage qualifier キーワード
var glslQualifiers = map[string]bool{
	"uniform":   true,
	"buffer":    true,
	"shared":    true,
	"in":        true,
	"out":       true,
	"inout":     true,
	"varying":   true,
	"attribute": true,
}

func (e *GLSLExtractor) Extract(src []byte) (*skeleton.File, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(glsl.GetLanguage())

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
		case "preproc_include":
			if imp := glslExtractInclude(child, src); imp != "" {
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

		case "declaration":
			exports := glslExtractDeclaration(child, src)
			file.Exports = append(file.Exports, exports...)

		case "function_definition":
			if exp := glslExtractFuncDef(child, src); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}
		}
	}

	return file, nil
}

// glslExtractInclude は #include から import パスを抽出する。
// GLSL の #include は string_literal を使う（system_lib_string ではない）。
func glslExtractInclude(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "string_literal":
			return unquote(content(child, src))
		case "system_lib_string":
			s := content(child, src)
			if len(s) >= 2 {
				return s[1 : len(s)-1]
			}
			return s
		}
	}
	return ""
}

func glslExtractDeclaration(node *sitter.Node, src []byte) []skeleton.Export {
	// ERROR ノードに巻き込まれた struct を救出
	var exports []skeleton.Export
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "ERROR" {
			exports = append(exports, glslExtractFromError(child, src)...)
		}
	}

	// ERROR 以降の有効範囲を特定
	startByte := node.StartByte()
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.IsNamed() && child.Type() == "ERROR" {
			startByte = child.EndByte()
		}
	}

	// qualifier を収集（ERROR 以降のみ）
	qualifier := glslCollectQualifier(node, src)

	// block declaration (uniform/buffer block with field_declaration_list)
	if fdl := cFindChildByType(node, "field_declaration_list"); fdl != nil {
		name := glslFindBlockName(node, src)
		if name != "" {
			members := cExtractStructFields(fdl, src)
			sig := qualifier + " " + name
			exports = append(exports, skeleton.Export{
				Kind:      skeleton.ExportClass,
				Name:      name,
				Signature: strings.TrimSpace(sig),
				Members:   members,
			})
			return exports
		}
	}

	// struct_specifier（トップレベル直接の struct）
	if ss := cFindChildByType(node, "struct_specifier"); ss != nil {
		if exp := cExtractStruct(ss, src); exp != nil {
			exports = append(exports, *exp)
		}
		return exports
	}

	// simple variable declaration (uniform float x; / in vec3 color; / shared vec4 buf[N]; 等)
	name := glslFindVarName(node, src)
	if name != "" {
		// ERROR 部分を除いたシグネチャを生成
		sig := strings.TrimSuffix(strings.TrimSpace(string(src[startByte:node.EndByte()])), ";")
		exports = append(exports, skeleton.Export{
			Kind:      skeleton.ExportVariable,
			Name:      name,
			Signature: sig,
		})
		return exports
	}

	return exports
}

// glslCollectQualifier は declaration の anonymous children から GLSL qualifier を収集する。
// layout(...) + uniform/buffer/shared/in/out をまとめた文字列を返す。
// ERROR ノードがある場合、ERROR 以降の子ノードからのみ収集する
// （layout(local_size_x=...) in; が後続を巻き込む問題への対策）。
func glslCollectQualifier(node *sitter.Node, src []byte) string {
	// ERROR ノードの位置を探す
	errorIdx := -1
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.IsNamed() && child.Type() == "ERROR" {
			errorIdx = i
		}
	}

	startIdx := 0
	if errorIdx >= 0 {
		startIdx = errorIdx + 1
	}

	var parts []string
	for i := startIdx; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.IsNamed() && child.Type() == "layout_specification" {
			parts = append(parts, content(child, src))
		}
		if !child.IsNamed() {
			text := content(child, src)
			if glslQualifiers[text] {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, " ")
}

// glslFindBlockName は uniform/buffer block の名前（identifier）を探す。
func glslFindBlockName(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "identifier" {
			return content(child, src)
		}
	}
	return ""
}

// glslFindVarName は declaration から変数名を探す。
// identifier, array_declarator, init_declarator のいずれかから名前を取る。
func glslFindVarName(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "identifier":
			name := content(child, src)
			// type_identifier や GLSL 型名ではない identifier が変数名
			if !glslQualifiers[name] {
				return name
			}
		case "array_declarator":
			return cInitDeclaratorName(child, src)
		case "init_declarator":
			return cInitDeclaratorName(child, src)
		}
	}
	return ""
}

// glslExtractFromError は ERROR ノードに巻き込まれた struct_specifier を救出する。
// layout(local_size_x = ...) in; が後続の struct を ERROR に巻き込む問題に対応。
func glslExtractFromError(node *sitter.Node, src []byte) []skeleton.Export {
	var exports []skeleton.Export
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "struct_specifier" {
			if exp := cExtractStruct(child, src); exp != nil {
				exports = append(exports, *exp)
			}
		}
	}
	return exports
}

func glslExtractFuncDef(node *sitter.Node, src []byte) *skeleton.Export {
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

	return &skeleton.Export{
		Kind:      skeleton.ExportFunction,
		Name:      name,
		Signature: sig,
	}
}
