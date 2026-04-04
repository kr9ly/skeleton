package lang

import "path/filepath"

type Language int

const (
	Unknown Language = iota
	TypeScript
	Markdown
)

func Detect(path string) Language {
	switch filepath.Ext(path) {
	case ".ts", ".tsx", ".mts", ".cts":
		return TypeScript
	case ".js", ".jsx", ".mjs", ".cjs":
		return TypeScript // same grammar
	case ".md":
		return Markdown
	default:
		return Unknown
	}
}
