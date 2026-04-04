package main

import (
	"fmt"
	"os"

	"github.com/kr9ly/skeleton/internal/extractor"
	"github.com/kr9ly/skeleton/internal/lang"
	"github.com/kr9ly/skeleton/internal/render"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: skeleton <file>")
		os.Exit(1)
	}

	path := os.Args[1]

	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	language := lang.Detect(path)
	if language == lang.Unknown {
		fmt.Fprintf(os.Stderr, "unsupported file type: %s\n", path)
		os.Exit(1)
	}

	var ext extractor.Extractor
	switch language {
	case lang.TypeScript:
		ext = extractor.NewTypeScript()
	default:
		fmt.Fprintf(os.Stderr, "unsupported language: %s\n", path)
		os.Exit(1)
	}

	file, err := ext.Extract(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		os.Exit(1)
	}
	file.Path = path

	fmt.Print(render.Text(file))
}
