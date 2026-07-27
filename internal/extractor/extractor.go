package extractor

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/kr9ly/skeleton/internal/lang"
	"github.com/kr9ly/skeleton/skeleton"
)

// nodeLines はノードの開始行・終了行（1-based）を返す。
// 一部の文法（tree-sitter-c の preproc_def 等）はノード終端が次行の列0に食い込むため、
// その場合は前の行までに丸める。
func nodeLines(node *sitter.Node) (int, int) {
	start := int(node.StartPoint().Row) + 1
	end := int(node.EndPoint().Row) + 1
	if node.EndPoint().Column == 0 && end > start {
		end--
	}
	return start, end
}

type Extractor interface {
	Extract(src []byte) (*skeleton.File, error)
}

func New(language lang.Language) Extractor {
	switch language {
	case lang.TypeScript:
		return NewTypeScript()
	case lang.Python:
		return NewPython()
	case lang.Go:
		return NewGo()
	case lang.Markdown:
		return NewMarkdown()
	case lang.Kotlin:
		return NewKotlin()
	case lang.C:
		return NewC()
	case lang.CUDA:
		return NewCUDA()
	case lang.Zig:
		return NewZig()
	case lang.CPP:
		return NewCPP()
	case lang.GLSL:
		return NewGLSL()
	case lang.Java:
		return NewJava()
	default:
		return nil
	}
}
