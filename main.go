package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/kr9ly/skeleton/internal/edit"
	"github.com/kr9ly/skeleton/internal/extractor"
	"github.com/kr9ly/skeleton/internal/lang"
	"github.com/kr9ly/skeleton/internal/mcp"
	"github.com/kr9ly/skeleton/internal/render"
	"github.com/kr9ly/skeleton/internal/scanner"
	"github.com/kr9ly/skeleton/internal/selector"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		if err := mcp.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "mcp error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "edit" {
		runEdit(os.Args[2:])
		return
	}

	fs := flag.NewFlagSet("skeleton", flag.ExitOnError)
	depth := fs.Int("depth", 1, "directory traversal depth")
	noTest := fs.Bool("no-test", false, "exclude test files")
	filter := fs.String("filter", "", "glob pattern to filter files (e.g. \"*.kt\")")
	fs.Parse(os.Args[1:])

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: skeleton [flags] <file|dir>")
		fmt.Fprintln(os.Stderr, "       skeleton edit <insert|remove> [flags] <file>")
		fs.PrintDefaults()
		os.Exit(1)
	}

	path := fs.Arg(0)
	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if info.IsDir() {
		opts := scanner.Options{Depth: *depth, NoTest: *noTest, Filter: *filter}
		dir, err := scanner.ScanDir(path, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(render.TextDir(dir))
	} else {
		runFile(path)
	}
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

	ext := newExtractor(language)
	if ext == nil {
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

func runEdit(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: skeleton edit <insert|remove> [flags] <file>")
		os.Exit(1)
	}

	subcmd := args[0]
	switch subcmd {
	case "insert":
		runEditInsert(args[1:])
	case "remove":
		runEditRemove(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown edit command: %s\n", subcmd)
		os.Exit(1)
	}
}

func runEditInsert(args []string) {
	fs := flag.NewFlagSet("skeleton edit insert", flag.ExitOnError)
	before := fs.String("before", "", "insert before this selector")
	after := fs.String("after", "", "insert after this selector")
	into := fs.String("into", "", "insert into body (end)")
	intoHead := fs.String("into-head", "", "insert into body (start)")
	dryRun := fs.Bool("dry-run", false, "print result to stdout instead of writing")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: skeleton edit insert --before|--after|--into|--into-head <selector> <file>")
		os.Exit(1)
	}

	path := fs.Arg(0)
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	code, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
		os.Exit(1)
	}
	codeStr := string(code)

	var selStr string
	var result []byte

	switch {
	case *before != "":
		selStr = *before
		sel, err := selector.Parse(selStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "selector error: %v\n", err)
			os.Exit(1)
		}
		result, err = edit.InsertBefore(src, path, sel, codeStr)
	case *after != "":
		selStr = *after
		sel, err := selector.Parse(selStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "selector error: %v\n", err)
			os.Exit(1)
		}
		result, err = edit.InsertAfter(src, path, sel, codeStr)
	case *into != "":
		selStr = *into
		sel, err := selector.Parse(selStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "selector error: %v\n", err)
			os.Exit(1)
		}
		result, err = edit.InsertInto(src, path, sel, codeStr, false)
	case *intoHead != "":
		selStr = *intoHead
		sel, err := selector.Parse(selStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "selector error: %v\n", err)
			os.Exit(1)
		}
		result, err = edit.InsertInto(src, path, sel, codeStr, true)
	default:
		fmt.Fprintln(os.Stderr, "specify one of: --before, --after, --into, --into-head")
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "edit error: %v\n", err)
		os.Exit(1)
	}

	if *dryRun {
		fmt.Print(string(result))
	} else {
		if err := os.WriteFile(path, result, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write error: %v\n", err)
			os.Exit(1)
		}
	}
}

func runEditRemove(args []string) {
	fs := flag.NewFlagSet("skeleton edit remove", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "print result to stdout instead of writing")
	fs.Parse(args)

	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage: skeleton edit remove <selector> <file>")
		os.Exit(1)
	}

	selStr := fs.Arg(0)
	path := fs.Arg(1)

	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	sel, err := selector.Parse(selStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "selector error: %v\n", err)
		os.Exit(1)
	}

	result, err := edit.Remove(src, path, sel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "edit error: %v\n", err)
		os.Exit(1)
	}

	if *dryRun {
		fmt.Print(string(result))
	} else {
		if err := os.WriteFile(path, result, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write error: %v\n", err)
			os.Exit(1)
		}
	}
}

func newExtractor(language lang.Language) extractor.Extractor {
	switch language {
	case lang.TypeScript:
		return extractor.NewTypeScript()
	case lang.Python:
		return extractor.NewPython()
	case lang.Go:
		return extractor.NewGo()
	case lang.Markdown:
		return extractor.NewMarkdown()
	case lang.Kotlin:
		return extractor.NewKotlin()
	default:
		return nil
	}
}
