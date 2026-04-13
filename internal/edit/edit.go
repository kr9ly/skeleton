package edit

import (
	"context"
	"fmt"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/kr9ly/skeleton/internal/lang"
	"github.com/kr9ly/skeleton/internal/selector"
)

func getLanguage(path string) *sitter.Language {
	switch lang.Detect(path) {
	case lang.TypeScript:
		ts := getLangTS()
		return ts
	case lang.Python:
		py := getLangPy()
		return py
	case lang.Go:
		return getLangGo()
	case lang.Kotlin:
		return getLangKotlin()
	case lang.C:
		return getLangC()
	case lang.CUDA:
		return getLangC() // CUDA は C パーサーで解析
	case lang.Zig:
		return getLangZig()
	case lang.CPP:
		return getLangCPP()
	case lang.GLSL:
		return getLangGLSL()
	default:
		return nil
	}
}

func parse(src []byte, language *sitter.Language) (*sitter.Tree, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(language)
	return parser.ParseCtx(context.Background(), nil, src)
}

// InsertBefore はセレクタで指定したノードの直前にコードを挿入する。
func InsertBefore(src []byte, path string, sel selector.Selector, code string) ([]byte, error) {
	language := getLanguage(path)
	if language == nil {
		return nil, fmt.Errorf("unsupported language: %s", path)
	}

	tree, err := parse(src, language)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	m, ok := selector.Find(sel, tree.RootNode(), src)
	if !ok {
		return nil, fmt.Errorf("selector not found")
	}

	// export_statement の場合は export 文全体の前に挿入
	target := findExportParent(m.Node, tree.RootNode())
	pos := target.StartByte()
	return splice(src, pos, pos, code+"\n"), nil
}

// InsertAfter はセレクタで指定したノードの直後にコードを挿入する。
func InsertAfter(src []byte, path string, sel selector.Selector, code string) ([]byte, error) {
	language := getLanguage(path)
	if language == nil {
		return nil, fmt.Errorf("unsupported language: %s", path)
	}

	tree, err := parse(src, language)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	m, ok := selector.Find(sel, tree.RootNode(), src)
	if !ok {
		return nil, fmt.Errorf("selector not found")
	}

	target := findExportParent(m.Node, tree.RootNode())
	pos := target.EndByte()
	return splice(src, pos, pos, "\n"+code), nil
}

// InsertInto はセレクタで指定したノードの body 末尾にコードを挿入する。
func InsertInto(src []byte, path string, sel selector.Selector, code string, head bool) ([]byte, error) {
	language := getLanguage(path)
	if language == nil {
		return nil, fmt.Errorf("unsupported language: %s", path)
	}

	tree, err := parse(src, language)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	m, ok := selector.Find(sel, tree.RootNode(), src)
	if !ok {
		return nil, fmt.Errorf("selector not found")
	}

	body := selector.FindBody(m.Node)
	if body == nil {
		return nil, fmt.Errorf("node has no body")
	}

	if head {
		// body の開き括弧の直後
		pos := body.StartByte() + 1
		return splice(src, pos, pos, "\n"+code), nil
	}
	// body の閉じ括弧の直前
	pos := body.EndByte() - 1
	return splice(src, pos, pos, code+"\n"), nil
}

// Remove はセレクタで指定したノードを削除する。
func Remove(src []byte, path string, sel selector.Selector) ([]byte, error) {
	language := getLanguage(path)
	if language == nil {
		return nil, fmt.Errorf("unsupported language: %s", path)
	}

	tree, err := parse(src, language)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	m, ok := selector.Find(sel, tree.RootNode(), src)
	if !ok {
		return nil, fmt.Errorf("selector not found")
	}

	target := findExportParent(m.Node, tree.RootNode())
	start := target.StartByte()
	end := target.EndByte()

	// 後続の改行も含めて削除
	if int(end) < len(src) && src[end] == '\n' {
		end++
	}

	return splice(src, start, end, ""), nil
}

// findExportParent は、ノードが export_statement の子なら export_statement を返す。
func findExportParent(node *sitter.Node, root *sitter.Node) *sitter.Node {
	parent := node.Parent()
	if parent != nil && parent.Type() == "export_statement" {
		return parent
	}
	return node
}

func splice(src []byte, start, end uint32, insert string) []byte {
	var result []byte
	result = append(result, src[:start]...)
	result = append(result, []byte(insert)...)
	result = append(result, src[end:]...)
	return result
}
