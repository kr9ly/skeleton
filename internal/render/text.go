package render

import (
	"fmt"
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

	writeFileBody(&b, f)

	return b.String()
}

func TextDir(d *skeleton.Dir) string {
	var b strings.Builder

	b.WriteString("# ")
	b.WriteString(d.Path)
	b.WriteString("/\n\n")

	for _, f := range d.Files {
		b.WriteString(f.Path)
		b.WriteString("\n")

		if len(f.Exports) > 0 {
			b.WriteString("  exports: ")
			for i, exp := range f.Exports {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(exp.Kind.String())
				b.WriteString(" ")
				if exp.Name != "" {
					b.WriteString(exp.Name)
				} else {
					b.WriteString("(re-export)")
				}
			}
			b.WriteString("\n")
		}

		// 外部 import は省略、ディレクトリ内の相対 import のみ表示
		var localImports []string
		for _, imp := range f.Imports {
			if strings.HasPrefix(imp, ".") {
				localImports = append(localImports, imp)
			}
		}
		if len(localImports) > 0 {
			b.WriteString("  imports: ")
			b.WriteString(strings.Join(localImports, ", "))
			b.WriteString("\n")
		}
	}

	if len(d.Deps) > 0 {
		b.WriteString("\n## deps\n")
		for _, dep := range d.Deps {
			fmt.Fprintf(&b, "%s <- %s\n", dep.Source, strings.Join(dep.Users, ", "))
		}
	}

	return b.String()
}

func writeFileBody(b *strings.Builder, f *skeleton.File) {
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
}
