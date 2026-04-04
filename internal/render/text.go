package render

import (
	"strings"

	"github.com/kr9ly/skeleton/skeleton"
)

func Text(f *skeleton.File) string {
	var b strings.Builder

	if f.Path != "" {
		b.WriteString("# ")
		b.WriteString(f.Path)
		b.WriteString("\n\n")
	}

	for _, imp := range f.Imports {
		b.WriteString("import ")
		b.WriteString(imp)
		b.WriteString("\n")
	}

	if len(f.Imports) > 0 && len(f.Exports) > 0 {
		b.WriteString("\n")
	}

	for _, exp := range f.Exports {
		b.WriteString("export ")
		b.WriteString(exp.Signature)
		b.WriteString("\n")
	}

	return b.String()
}
