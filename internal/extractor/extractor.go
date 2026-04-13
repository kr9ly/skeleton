package extractor

import (
	"github.com/kr9ly/skeleton/internal/lang"
	"github.com/kr9ly/skeleton/skeleton"
)

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
	default:
		return nil
	}
}
