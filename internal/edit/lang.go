package edit

import (
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

func getLangTS() *sitter.Language {
	return typescript.GetLanguage()
}

func getLangPy() *sitter.Language {
	return python.GetLanguage()
}
