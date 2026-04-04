package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kr9ly/skeleton/internal/extractor"
	"github.com/kr9ly/skeleton/internal/lang"
	"github.com/kr9ly/skeleton/internal/tsconfig"
	"github.com/kr9ly/skeleton/skeleton"
)

type Options struct {
	Depth  int
	NoTest bool
}

func ScanDir(dir string, opts Options) (*skeleton.Dir, error) {
	dir = filepath.Clean(dir)

	var files []skeleton.File
	err := walkDir(dir, dir, opts, func(path string, src []byte) {
		language := lang.Detect(path)
		if language == lang.Unknown {
			return
		}

		var ext extractor.Extractor
		switch language {
		case lang.TypeScript:
			ext = extractor.NewTypeScript()
		case lang.Python:
			ext = extractor.NewPython()
		case lang.Go:
			ext = extractor.NewGo()
		default:
			return
		}

		f, err := ext.Extract(src)
		if err != nil {
			return
		}

		rel, _ := filepath.Rel(dir, path)
		f.Path = rel
		files = append(files, *f)
	})
	if err != nil {
		return nil, err
	}

	aliases := tsconfig.FindAndLoad(dir)
	deps := buildDeps(dir, files, aliases)

	return &skeleton.Dir{
		Path:  dir,
		Files: files,
		Deps:  deps,
	}, nil
}

func walkDir(base, dir string, opts Options, fn func(path string, src []byte)) error {
	if opts.Depth < 0 {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	childOpts := Options{Depth: opts.Depth - 1, NoTest: opts.NoTest}

	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.IsDir() {
			if shouldSkipDir(e.Name()) {
				continue
			}
			if err := walkDir(base, path, childOpts, fn); err != nil {
				return err
			}
		} else {
			if lang.Detect(path) == lang.Unknown {
				continue
			}
			if opts.NoTest && isTestFile(e.Name()) {
				continue
			}
			src, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			fn(path, src)
		}
	}
	return nil
}

func shouldSkipDir(name string) bool {
	return name == "node_modules" || name == ".git" || name == "dist" || name == "build" || name == "vendor" || strings.HasPrefix(name, ".")
}

func isTestFile(name string) bool {
	return strings.HasSuffix(name, ".test.ts") ||
		strings.HasSuffix(name, ".test.tsx") ||
		strings.HasSuffix(name, ".spec.ts") ||
		strings.HasSuffix(name, ".spec.tsx") ||
		strings.HasSuffix(name, ".test.js") ||
		strings.HasSuffix(name, ".test.jsx") ||
		strings.HasSuffix(name, ".spec.js") ||
		strings.HasSuffix(name, ".spec.jsx") ||
		strings.HasPrefix(name, "test_") ||
		strings.HasSuffix(name, "_test.py") ||
		name == "conftest.py" ||
		strings.HasSuffix(name, "_test.go")
}

func buildDeps(baseDir string, files []skeleton.File, aliases *tsconfig.PathAliases) []skeleton.Dep {
	absBaseDir, _ := filepath.Abs(baseDir)

	// ディレクトリ内のファイルパスを集める（拡張子なし → 相対パス）
	fileSet := make(map[string]string) // 拡張子なしパス → 相対パス
	// 絶対パス版（alias 解決用）
	absFileSet := make(map[string]string) // 絶対パス（拡張子なし）→ 相対パス
	for _, f := range files {
		noExt := strings.TrimSuffix(f.Path, filepath.Ext(f.Path))
		fileSet[noExt] = f.Path
		absFileSet[filepath.Join(absBaseDir, noExt)] = f.Path
		// index ファイルはディレクトリ名でも参照可能
		if filepath.Base(noExt) == "index" {
			fileSet[filepath.Dir(noExt)] = f.Path
			absFileSet[filepath.Join(absBaseDir, filepath.Dir(noExt))] = f.Path
		}
	}

	// import → どのファイルが使っているか
	usersOf := make(map[string][]string) // source → []user

	for _, f := range files {
		fileDir := filepath.Dir(f.Path)
		for _, imp := range f.Imports {
			var target string
			if strings.HasPrefix(imp, ".") {
				// 相対 import
				resolved := filepath.Clean(filepath.Join(fileDir, imp))
				target = fileSet[resolved]
			} else if abs := aliases.Resolve(imp); abs != "" {
				// path alias import
				target = absFileSet[abs]
			}
			if target != "" && target != f.Path {
				usersOf[target] = append(usersOf[target], f.Path)
			}
		}
	}

	var deps []skeleton.Dep
	for source, users := range usersOf {
		sort.Strings(users)
		deps = append(deps, skeleton.Dep{Source: source, Users: users})
	}
	sort.Slice(deps, func(i, j int) bool {
		return len(deps[i].Users) > len(deps[j].Users) // 被依存数の多い順
	})

	return deps
}
