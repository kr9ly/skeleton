package tsconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type PathAliases struct {
	baseDir string            // tsconfig.json のあるディレクトリ
	baseURL string            // compilerOptions.baseUrl（絶対パス化済み）
	paths   map[string]string // prefix("@/") → replacement dir（絶対パス）
}

type tsconfigJSON struct {
	CompilerOptions struct {
		BaseURL string              `json:"baseUrl"`
		Paths   map[string][]string `json:"paths"`
	} `json:"compilerOptions"`
}

// FindAndLoad は dir から親方向に tsconfig.json を探し、paths を読み込む。
// 見つからなければ nil を返す（エラーではない）。
func FindAndLoad(dir string) *PathAliases {
	dir, _ = filepath.Abs(dir)
	for {
		path := filepath.Join(dir, "tsconfig.json")
		if data, err := os.ReadFile(path); err == nil {
			if aliases := parse(dir, data); aliases != nil {
				return aliases
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

func parse(baseDir string, data []byte) *PathAliases {
	var cfg tsconfigJSON
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}

	if len(cfg.CompilerOptions.Paths) == 0 {
		return nil
	}

	baseURL := baseDir
	if cfg.CompilerOptions.BaseURL != "" {
		baseURL = filepath.Join(baseDir, cfg.CompilerOptions.BaseURL)
	}

	aliases := &PathAliases{
		baseDir: baseDir,
		baseURL: baseURL,
		paths:   make(map[string]string),
	}

	for pattern, targets := range cfg.CompilerOptions.Paths {
		if len(targets) == 0 {
			continue
		}
		// "@/*" → prefix "@/", target "./src/*" → dir
		prefix := strings.TrimSuffix(pattern, "*")
		target := strings.TrimSuffix(targets[0], "*")
		resolved := filepath.Join(baseURL, target)
		aliases.paths[prefix] = resolved
	}

	return aliases
}

// Resolve は path alias を絶対パスに解決する。
// alias にマッチしなければ空文字を返す。
func (a *PathAliases) Resolve(importPath string) string {
	if a == nil {
		return ""
	}
	for prefix, dir := range a.paths {
		if strings.HasPrefix(importPath, prefix) {
			rest := importPath[len(prefix):]
			return filepath.Join(dir, rest)
		}
	}
	return ""
}
