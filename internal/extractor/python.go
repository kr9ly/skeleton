package extractor

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"

	"github.com/kr9ly/skeleton/skeleton"
)

type PythonExtractor struct{}

func NewPython() *PythonExtractor {
	return &PythonExtractor{}
}

func (e *PythonExtractor) Extract(src []byte) (*skeleton.File, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(python.GetLanguage())

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
			for _, imp := range pyExtractImport(child, src) {
				if !seen[imp] {
					seen[imp] = true
					file.Imports = append(file.Imports, imp)
				}
			}
		case "import_from_statement":
			if imp := pyExtractFromImport(child, src); imp != "" {
				if !seen[imp] {
					seen[imp] = true
					file.Imports = append(file.Imports, imp)
				}
			}
		case "future_import_statement":
			// skip __future__ imports
		case "function_definition":
			if exp := pyExtractFunction(child, src, ""); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}
		case "class_definition":
			if exp := pyExtractClass(child, src, ""); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}
		case "decorated_definition":
			if exp := pyExtractDecorated(child, src); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}
		case "expression_statement":
			if exp := pyExtractAssignment(child, src); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}
		case "type_alias_statement":
			if exp := pyExtractTypeAlias(child, src); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}
		}
	}

	return file, nil
}

func pyExtractImport(node *sitter.Node, src []byte) []string {
	var imports []string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "dotted_name":
			imports = append(imports, content(child, src))
		case "aliased_import":
			if name := child.ChildByFieldName("name"); name != nil {
				imports = append(imports, content(name, src))
			}
		}
	}
	return imports
}

func pyExtractFromImport(node *sitter.Node, src []byte) string {
	moduleName := node.ChildByFieldName("module_name")
	if moduleName == nil {
		return ""
	}
	mod := content(moduleName, src)
	// 相対 import（"." や "..core"）はそのまま使える
	// 絶対 import は "from typing import ..." → "typing" でよい
	return mod
}

func pyExtractFunction(node *sitter.Node, src []byte, decorator string) *skeleton.Export {
	name := fieldContent(node, "name", src)
	if name == "" || strings.HasPrefix(name, "_") {
		return nil
	}

	sig := pyFunctionSignature(node, src)
	if decorator != "" {
		sig = decorator + "\n" + sig
	}

	return &skeleton.Export{
		Kind:      skeleton.ExportFunction,
		Name:      name,
		Signature: sig,
	}
}

func pyFunctionSignature(node *sitter.Node, src []byte) string {
	name := fieldContent(node, "name", src)
	params := fieldContent(node, "parameters", src)
	returnType := node.ChildByFieldName("return_type")

	sig := "def " + name + params
	if returnType != nil {
		sig += " -> " + content(returnType, src)
	}
	return sig
}

func pyExtractClass(node *sitter.Node, src []byte, decorator string) *skeleton.Export {
	name := fieldContent(node, "name", src)
	if name == "" || strings.HasPrefix(name, "_") {
		return nil
	}

	sig := "class " + name
	if supers := node.ChildByFieldName("superclasses"); supers != nil {
		sig += content(supers, src)
	}

	if decorator != "" {
		sig = decorator + "\n" + sig
	}

	return &skeleton.Export{
		Kind:      skeleton.ExportClass,
		Name:      name,
		Signature: sig,
	}
}

func pyExtractDecorated(node *sitter.Node, src []byte) *skeleton.Export {
	// デコレーター文字列を収集
	var decorators []string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "decorator" {
			decorators = append(decorators, content(child, src))
		}
	}
	decoStr := strings.Join(decorators, "\n")

	defNode := node.ChildByFieldName("definition")
	if defNode == nil {
		return nil
	}

	switch defNode.Type() {
	case "function_definition":
		return pyExtractFunction(defNode, src, decoStr)
	case "class_definition":
		return pyExtractClass(defNode, src, decoStr)
	default:
		return nil
	}
}

func pyExtractAssignment(node *sitter.Node, src []byte) *skeleton.Export {
	// expression_statement の子が assignment かチェック
	if node.NamedChildCount() < 1 {
		return nil
	}
	assign := node.NamedChild(0)
	if assign.Type() != "assignment" {
		return nil
	}

	left := assign.ChildByFieldName("left")
	if left == nil || left.Type() != "identifier" {
		return nil
	}

	name := content(left, src)
	if strings.HasPrefix(name, "_") && name != "__all__" {
		return nil
	}

	// __all__ は特別扱い（export 一覧として情報になる）
	if name == "__all__" {
		right := assign.ChildByFieldName("right")
		if right != nil {
			return &skeleton.Export{
				Kind:      skeleton.ExportVariable,
				Name:      name,
				Signature: "__all__ = " + content(right, src),
			}
		}
		return nil
	}

	// 型注釈があればそれを含める
	typeNode := assign.ChildByFieldName("type")
	if typeNode != nil {
		return &skeleton.Export{
			Kind:      skeleton.ExportVariable,
			Name:      name,
			Signature: name + ": " + content(typeNode, src),
		}
	}

	// 大文字始まりの定数のみ（小文字の普通の変数は除外）
	if name[0] >= 'A' && name[0] <= 'Z' {
		return &skeleton.Export{
			Kind:      skeleton.ExportVariable,
			Name:      name,
			Signature: name,
		}
	}

	return nil
}

func pyExtractTypeAlias(node *sitter.Node, src []byte) *skeleton.Export {
	name := fieldContent(node, "name", src)
	if name == "" || strings.HasPrefix(name, "_") {
		return nil
	}
	return &skeleton.Export{
		Kind:      skeleton.ExportType,
		Name:      name,
		Signature: strings.TrimSpace(content(node, src)),
	}
}
