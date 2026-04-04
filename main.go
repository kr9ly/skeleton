package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kr9ly/skeleton/internal/extractor"
	"github.com/kr9ly/skeleton/internal/lang"
	"github.com/kr9ly/skeleton/internal/render"
	"github.com/kr9ly/skeleton/internal/scanner"
)

var (
	depth  = flag.Int("depth", 1, "directory traversal depth")
	noTest = flag.Bool("no-test", false, "exclude test files (*.test.ts, *.spec.ts)")
)

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: skeleton [flags] <file|dir>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	path := flag.Arg(0)

	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if info.IsDir() {
		runDir(path)
	} else {
		runFile(path)
	}
}

func runDir(path string) {
	opts := scanner.Options{
		Depth:  *depth,
		NoTest: *noTest,
	}
	dir, err := scanner.ScanDir(path, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(render.TextDir(dir))
}

func runFile(path string) {
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
