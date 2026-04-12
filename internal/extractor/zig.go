package extractor

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/kr9ly/skeleton/internal/treesitter/zig"
	"github.com/kr9ly/skeleton/skeleton"
)

type ZigExtractor struct{}

func NewZig() *ZigExtractor {
	return &ZigExtractor{}
}

func (e *ZigExtractor) Extract(src []byte) (*skeleton.File, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(zig.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	root := tree.RootNode()
	file := &skeleton.File{}
	seen := make(map[string]bool)

	// All children (named + anonymous) を走査して pub を検出
	isPub := false
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)

		if !child.IsNamed() && child.Type() == "pub" {
			isPub = true
			continue
		}

		if child.Type() == "Decl" {
			zigProcessDecl(child, src, isPub, file, seen)
			isPub = false
			continue
		}

		isPub = false
	}

	return file, nil
}

func zigProcessDecl(node *sitter.Node, src []byte, isPub bool, file *skeleton.File, seen map[string]bool) {
	// Decl の named children: VarDecl or (FnProto + Block)
	varDecl := zigFindNamedChild(node, "VarDecl")
	if varDecl != nil {
		zigProcessVarDecl(varDecl, src, isPub, file, seen)
		return
	}

	fnProto := zigFindNamedChild(node, "FnProto")
	if fnProto != nil && isPub {
		if exp := zigExtractFnProto(fnProto, src); exp != nil {
			file.Exports = append(file.Exports, *exp)
		}
	}
}

func zigProcessVarDecl(node *sitter.Node, src []byte, isPub bool, file *skeleton.File, seen map[string]bool) {
	name := zigVarDeclName(node, src)
	if name == "" {
		return
	}

	isVar := zigIsVar(node)
	hasTypeAnnotation := zigHasTypeAnnotation(node)

	// 値の式ノードを取得
	valueText := zigValueText(node, src)

	// @import の検出
	if strings.HasPrefix(valueText, "@import(") {
		imp := zigExtractImportPath(valueText)
		if imp != "" && !seen[imp] {
			seen[imp] = true
			file.Imports = append(file.Imports, imp)
		}
		if !isPub {
			return
		}
	}

	if !isPub {
		return
	}

	// ContainerDecl (struct/enum/union) の検出
	containerNode := zigFindContainerDecl(node)
	if containerNode != nil {
		kind, kindName := zigContainerKind(containerNode, src)
		members := zigExtractContainerMembers(containerNode, src)

		exportKind := skeleton.ExportType
		if kind == "struct" {
			exportKind = skeleton.ExportClass
		}

		file.Exports = append(file.Exports, skeleton.Export{
			Kind:      exportKind,
			Name:      name,
			Signature: kindName + " " + name,
			Members:   members,
		})
		return
	}

	// error set の検出
	if strings.HasPrefix(valueText, "error{") {
		file.Exports = append(file.Exports, skeleton.Export{
			Kind:      skeleton.ExportType,
			Name:      name,
			Signature: "error " + name,
		})
		return
	}

	// var → 変数
	if isVar {
		sig := "var " + name
		if typeText := zigTypeAnnotationText(node, src); typeText != "" {
			sig += ": " + typeText
		}
		file.Exports = append(file.Exports, skeleton.Export{
			Kind:      skeleton.ExportVariable,
			Name:      name,
			Signature: sig,
		})
		return
	}

	// 型注釈あり → 定数
	if hasTypeAnnotation {
		typeText := zigTypeAnnotationText(node, src)
		sig := "const " + name + ": " + typeText
		file.Exports = append(file.Exports, skeleton.Export{
			Kind:      skeleton.ExportVariable,
			Name:      name,
			Signature: sig,
		})
		return
	}

	// それ以外の const → 型エイリアス
	sig := "const " + name + " = " + valueText
	file.Exports = append(file.Exports, skeleton.Export{
		Kind:      skeleton.ExportType,
		Name:      name,
		Signature: sig,
	})
}

func zigExtractFnProto(node *sitter.Node, src []byte) *skeleton.Export {
	name := ""
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "IDENTIFIER" {
			name = content(child, src)
			break
		}
	}
	if name == "" {
		return nil
	}

	sig := "fn " + strings.TrimSpace(content(node, src))[3:] // "fn " + rest
	return &skeleton.Export{
		Kind:      skeleton.ExportFunction,
		Name:      name,
		Signature: sig,
	}
}

func zigExtractContainerMembers(node *sitter.Node, src []byte) []skeleton.Member {
	var members []skeleton.Member

	isPub := false
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)

		if !child.IsNamed() && child.Type() == "pub" {
			isPub = true
			continue
		}

		switch child.Type() {
		case "ContainerField":
			name := ""
			for j := 0; j < int(child.NamedChildCount()); j++ {
				gc := child.NamedChild(j)
				if gc.Type() == "IDENTIFIER" {
					name = content(gc, src)
					break
				}
			}
			sig := strings.TrimSpace(content(child, src))
			if name == "" {
				// enum variant: IDENTIFIER がない場合はテキストから取得
				name = sig
			}
			if name != "" {
				members = append(members, skeleton.Member{
					Kind:      skeleton.MemberField,
					Name:      name,
					Signature: sig,
				})
			}
			isPub = false

		case "Decl":
			if !isPub {
				isPub = false
				continue
			}
			// pub メソッドまたはネストされた型
			fnProto := zigFindNamedChild(child, "FnProto")
			if fnProto != nil {
				name := ""
				for j := 0; j < int(fnProto.NamedChildCount()); j++ {
					gc := fnProto.NamedChild(j)
					if gc.Type() == "IDENTIFIER" {
						name = content(gc, src)
						break
					}
				}
				if name != "" {
					sig := "fn " + strings.TrimSpace(content(fnProto, src))[3:]
					members = append(members, skeleton.Member{
						Kind:      skeleton.MemberMethod,
						Name:      name,
						Signature: sig,
					})
				}
			} else {
				varDecl := zigFindNamedChild(child, "VarDecl")
				if varDecl != nil {
					name := zigVarDeclName(varDecl, src)
					if name != "" {
						members = append(members, skeleton.Member{
							Kind:      skeleton.MemberField,
							Name:      name,
							Signature: "const " + name,
						})
					}
				}
			}
			isPub = false

		default:
			if child.IsNamed() {
				isPub = false
			}
		}
	}

	return members
}

// --- helpers ---

func zigFindNamedChild(node *sitter.Node, typeName string) *sitter.Node {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == typeName {
			return child
		}
	}
	return nil
}

func zigVarDeclName(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "IDENTIFIER" {
			return content(child, src)
		}
	}
	return ""
}

func zigIsVar(node *sitter.Node) bool {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "var" {
			return true
		}
		if child.Type() == "const" {
			return false
		}
	}
	return false
}

func zigHasTypeAnnotation(node *sitter.Node) bool {
	// IDENTIFIER の後に `:` があるか
	foundName := false
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.IsNamed() && child.Type() == "IDENTIFIER" {
			foundName = true
			continue
		}
		if foundName && !child.IsNamed() && child.Type() == ":" {
			return true
		}
		if foundName && !child.IsNamed() && child.Type() == "=" {
			return false
		}
	}
	return false
}

func zigTypeAnnotationText(node *sitter.Node, src []byte) string {
	// `:` と `=` の間のノードテキスト
	foundColon := false
	start := uint32(0)
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if !child.IsNamed() && child.Type() == ":" {
			foundColon = true
			start = child.EndByte()
			continue
		}
		if foundColon && !child.IsNamed() && child.Type() == "=" {
			return strings.TrimSpace(string(src[start:child.StartByte()]))
		}
		if foundColon && !child.IsNamed() && child.Type() == ";" {
			return strings.TrimSpace(string(src[start:child.StartByte()]))
		}
	}
	return ""
}

func zigValueText(node *sitter.Node, src []byte) string {
	// `=` の後から `;` の前まで
	foundEq := false
	start := uint32(0)
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if !child.IsNamed() && child.Type() == "=" {
			foundEq = true
			start = child.EndByte()
			continue
		}
		if foundEq && !child.IsNamed() && child.Type() == ";" {
			return strings.TrimSpace(string(src[start:child.StartByte()]))
		}
	}
	if foundEq {
		return strings.TrimSpace(string(src[start:node.EndByte()]))
	}
	return ""
}

func zigExtractImportPath(valueText string) string {
	// @import("std") → std
	// @import("std").mem → std
	idx := strings.Index(valueText, "(\"")
	if idx < 0 {
		return ""
	}
	rest := valueText[idx+2:]
	end := strings.Index(rest, "\")")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func zigFindContainerDecl(node *sitter.Node) *sitter.Node {
	// VarDecl → ErrorUnionExpr → SuffixExpr → ContainerDecl
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "ErrorUnionExpr" {
			return zigFindContainerDeclIn(child)
		}
	}
	return nil
}

func zigFindContainerDeclIn(node *sitter.Node) *sitter.Node {
	if node.Type() == "ContainerDecl" {
		return node
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if found := zigFindContainerDeclIn(child); found != nil {
			return found
		}
	}
	return nil
}

func zigContainerKind(node *sitter.Node, src []byte) (kind string, kindName string) {
	declType := zigFindNamedChild(node, "ContainerDeclType")
	if declType == nil {
		return "struct", "struct"
	}
	for i := 0; i < int(declType.ChildCount()); i++ {
		child := declType.Child(i)
		t := child.Type()
		if t == "struct" || t == "enum" || t == "union" {
			return t, t
		}
	}
	return "struct", "struct"
}
