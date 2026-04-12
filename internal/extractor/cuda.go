package extractor

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/c"

	"github.com/kr9ly/skeleton/skeleton"
)

// CUDAExtractor は C パーサーを使って .cu/.cuh ファイルを解析する。
// CUDA 修飾子（__global__, __device__ 等）による AST のゴミをフィルタする。
type CUDAExtractor struct{}

func NewCUDA() *CUDAExtractor {
	return &CUDAExtractor{}
}

var cudaQualifiers = map[string]bool{
	"__global__":   true,
	"__device__":   true,
	"__host__":     true,
	"__shared__":   true,
	"__constant__": true,
	"__managed__":  true,
	"__noinline__": true,
	"__forceinline__": true,
}

func (e *CUDAExtractor) Extract(src []byte) (*skeleton.File, error) {
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

	// 関数定義の名前を集めて重複排除
	definedFuncs := make(map[string]bool)
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if child.Type() == "function_definition" {
			if cudaNodeIsStatic(child, src) {
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
			if cudaNodeIsStatic(child, src) {
				continue
			}
			if cudaIsQualifierOnlyDecl(child, src) {
				continue
			}
			exports := cudaExtractDeclaration(child, src, definedFuncs)
			file.Exports = append(file.Exports, exports...)

		case "function_definition":
			if cudaNodeIsStatic(child, src) {
				continue
			}
			if exp := cudaExtractFuncDef(child, src); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}
		}
	}

	return file, nil
}

// cudaIsQualifierOnlyDecl は `__host__ __device__` のような
// CUDA 修飾子だけの宣言（パーサーのゴミ）を検出する。
func cudaIsQualifierOnlyDecl(node *sitter.Node, src []byte) bool {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		name := content(child, src)
		switch child.Type() {
		case "type_identifier":
			if !cudaQualifiers[name] {
				return false
			}
		case "identifier":
			if !cudaQualifiers[name] {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// cudaNodeIsStatic は static 修飾を持つかチェックする。
// CUDA 修飾子が storage_class_specifier の手前にある場合も対応。
func cudaNodeIsStatic(node *sitter.Node, src []byte) bool {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "storage_class_specifier" && content(child, src) == "static" {
			return true
		}
	}
	return false
}

func cudaExtractFuncDef(node *sitter.Node, src []byte) *skeleton.Export {
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

func cudaExtractDeclaration(node *sitter.Node, src []byte, definedFuncs map[string]bool) []skeleton.Export {
	// 関数プロトタイプ
	if fd := cFindFuncDeclarator(node); fd != nil {
		name := cFuncDeclaratorName(fd, src)
		if name == "" || definedFuncs[name] {
			return nil
		}
		sig := strings.TrimSuffix(strings.TrimSpace(content(node, src)), ";")
		return []skeleton.Export{{
			Kind:      skeleton.ExportFunction,
			Name:      name,
			Signature: sig,
		}}
	}

	// CUDA 修飾付き変数（__constant__, __shared__）
	// 例: __constant__ float scale_factor; → type_identifier, ERROR, identifier
	// 例: __shared__ float shared_buffer[256]; → type_identifier, ERROR, array_declarator
	firstType := ""
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "type_identifier" {
			firstType = content(child, src)
			break
		}
	}

	if cudaQualifiers[firstType] {
		// CUDA 修飾付き変数: 名前を identifier か array_declarator から探す
		name := ""
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			switch child.Type() {
			case "identifier":
				n := content(child, src)
				if !cudaQualifiers[n] {
					name = n
				}
			case "array_declarator":
				name = cInitDeclaratorName(child, src)
			case "init_declarator":
				name = cInitDeclaratorName(child, src)
			}
		}
		if name != "" {
			sig := strings.TrimSuffix(strings.TrimSpace(content(node, src)), ";")
			return []skeleton.Export{{
				Kind:      skeleton.ExportVariable,
				Name:      name,
				Signature: sig,
			}}
		}
		return nil
	}

	// 通常の変数宣言: C と同じ
	return cExtractDeclaration(node, src, definedFuncs)
}
