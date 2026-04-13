package lang

import "path/filepath"

type Language int

const (
	Unknown Language = iota
	TypeScript
	Python
	Go
	Markdown
	Kotlin
	C
	CUDA
	Zig
	CPP
)

func Detect(path string) Language {
	switch filepath.Ext(path) {
	case ".ts", ".tsx", ".mts", ".cts":
		return TypeScript
	case ".js", ".jsx", ".mjs", ".cjs":
		return TypeScript // same grammar
	case ".py":
		return Python
	case ".go":
		return Go
	case ".md":
		return Markdown
	case ".kt", ".kts":
		return Kotlin
	case ".c", ".h":
		return C
	case ".cu", ".cuh":
		return CUDA
	case ".zig":
		return Zig
	case ".cpp", ".hpp", ".cc", ".hh", ".cxx", ".hxx":
		return CPP
	default:
		return Unknown
	}
}
