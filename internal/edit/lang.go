package edit

import (
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	clang "github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/kotlin"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
	glsllang "github.com/kr9ly/skeleton/internal/treesitter/glsl"
	ziglang "github.com/kr9ly/skeleton/internal/treesitter/zig"
)

func getLangTS() *sitter.Language {
	return typescript.GetLanguage()
}

func getLangPy() *sitter.Language {
	return python.GetLanguage()
}

func getLangGo() *sitter.Language {
	return golang.GetLanguage()
}

func getLangKotlin() *sitter.Language {
	return kotlin.GetLanguage()
}

func getLangC() *sitter.Language {
	return clang.GetLanguage()
}

func getLangZig() *sitter.Language {
	return ziglang.GetLanguage()
}

func getLangCPP() *sitter.Language {
	return cpp.GetLanguage()
}

func getLangGLSL() *sitter.Language {
	return glsllang.GetLanguage()
}
