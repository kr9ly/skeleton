package edit

import (
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
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
