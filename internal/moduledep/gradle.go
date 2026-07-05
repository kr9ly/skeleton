package moduledep

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/kr9ly/skeleton/skeleton"
)

// gradleProvider は Gradle マルチモジュールプロジェクトの依存グラフを抽出する。
// settings.gradle(.kts) の include 宣言でモジュールを列挙し、
// 各モジュールの build.gradle(.kts) から project() 依存を正規表現で拾う。
type gradleProvider struct{}

func (g *gradleProvider) Name() string { return "gradle" }

func (g *gradleProvider) Detect(root string) bool {
	return findSettingsFile(root) != ""
}

func findSettingsFile(root string) string {
	for _, name := range []string{"settings.gradle.kts", "settings.gradle"} {
		p := filepath.Join(root, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

var (
	// include ':app', ':core:model' / include(":app", ":core:model")
	// includeBuild は composite build なので除外
	includeRe = regexp.MustCompile(`(?m)^\s*include\b`)
	quotedRe  = regexp.MustCompile(`["']([^"']+)["']`)
	// project(":x").projectDir = file("custom/dir")
	projectDirRe = regexp.MustCompile(`project\s*\(\s*["']([^"']+)["']\s*\)\s*\.projectDir\s*=\s*(?:new\s+File|file)\s*\(\s*["']([^"']+)["']`)
	// implementation(project(":x")) / api project(':x') / implementation(project(path = ":x"))
	projectDepRe = regexp.MustCompile(`(\w+)\s*\(?\s*project\s*\(\s*(?:path\s*[:=]\s*)?["']([^"']+)["']`)
	// implementation(projects.coreModel) — typesafe project accessor
	accessorDepRe = regexp.MustCompile(`(\w+)\s*\(?\s*projects\.([A-Za-z0-9_.]+)`)
	// apply from: rootProject.file('settings/x.gradle') / apply(from = "${rootDir}/x.gradle")
	applyFromRe = regexp.MustCompile(`(?m)^\s*apply\s*\(?\s*from\s*[:=][^\n]*`)

	blockCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
	lineCommentRe  = regexp.MustCompile(`(?m)(^|[^:])//.*$`)
)

func stripComments(src string) string {
	src = blockCommentRe.ReplaceAllString(src, "")
	return lineCommentRe.ReplaceAllString(src, "$1")
}

func (g *gradleProvider) Extract(root string) (*skeleton.ModuleGraph, error) {
	settingsPath := findSettingsFile(root)
	src, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil, err
	}
	settings := stripComments(string(src))

	names := parseIncludes(settings)
	dirOverrides := parseProjectDirs(settings)

	// typesafe accessor（projects.coreModel）→ モジュール名の逆引き表
	accessorMap := make(map[string]string)
	for _, name := range names {
		accessorMap[accessorFor(name)] = name
	}

	var modules []skeleton.Module
	for _, name := range names {
		dir := dirOverrides[name]
		if dir == "" {
			dir = strings.ReplaceAll(strings.TrimPrefix(name, ":"), ":", string(filepath.Separator))
		}
		modules = append(modules, skeleton.Module{
			Name: name,
			Dir:  dir,
			Deps: parseDeps(root, filepath.Join(root, dir), accessorMap),
		})
	}

	// ルート自身が project 依存を持つ場合のみ ":" として含める
	if rootDeps := parseDeps(root, root, accessorMap); len(rootDeps) > 0 {
		modules = append(modules, skeleton.Module{Name: ":", Dir: ".", Deps: rootDeps})
	}

	sort.Slice(modules, func(i, j int) bool { return modules[i].Name < modules[j].Name })

	return &skeleton.ModuleGraph{Root: root, System: "gradle", Modules: modules}, nil
}

// parseIncludes は include 文からモジュール名を集める。
// include 直後から、引用文字列・カンマ・括弧・空白が続く範囲を1文とみなす（複数行対応）。
func parseIncludes(settings string) []string {
	var names []string
	seen := make(map[string]bool)

	for _, loc := range includeRe.FindAllStringIndex(settings, -1) {
		rest := settings[loc[1]:]
		end := includeStatementEnd(rest)
		for _, m := range quotedRe.FindAllStringSubmatch(rest[:end], -1) {
			name := normalizeModuleName(m[1])
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}

// includeStatementEnd は include 文の終端位置を返す。
// 空白・カンマ・括弧・引用文字列以外のトークンが現れたら終わり。
func includeStatementEnd(s string) int {
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == ',' || c == '(' || c == ')':
			i++
		case c == '"' || c == '\'':
			j := strings.IndexByte(s[i+1:], c)
			if j < 0 {
				return i
			}
			i += j + 2
		default:
			return i
		}
	}
	return i
}

func parseProjectDirs(settings string) map[string]string {
	dirs := make(map[string]string)
	for _, m := range projectDirRe.FindAllStringSubmatch(settings, -1) {
		dirs[normalizeModuleName(m[1])] = filepath.Clean(m[2])
	}
	return dirs
}

func normalizeModuleName(name string) string {
	if !strings.HasPrefix(name, ":") {
		return ":" + name
	}
	return name
}

// accessorFor は ":core:my-model" → "core.myModel" のように
// typesafe accessor 表記へ変換する（kebab-case / snake_case → camelCase）
func accessorFor(name string) string {
	segs := strings.Split(strings.TrimPrefix(name, ":"), ":")
	for i, seg := range segs {
		segs[i] = camelize(seg)
	}
	return strings.Join(segs, ".")
}

func camelize(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' })
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
}

// parseDeps は moduleDir の build.gradle(.kts) から project 依存を抽出する。
// apply from: で参照される共有スクリプト内の宣言も再帰的に辿る。
func parseDeps(root, moduleDir string, accessorMap map[string]string) []skeleton.ModuleDep {
	var buildFile string
	for _, name := range []string{"build.gradle.kts", "build.gradle"} {
		p := filepath.Join(moduleDir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			buildFile = p
			break
		}
	}
	if buildFile == "" {
		return nil
	}

	seen := make(map[skeleton.ModuleDep]bool)
	var deps []skeleton.ModuleDep
	add := func(kind, target string) {
		d := skeleton.ModuleDep{Target: target, Kind: kind}
		if !seen[d] {
			seen[d] = true
			deps = append(deps, d)
		}
	}

	visited := make(map[string]bool)
	var scan func(scriptPath string)
	scan = func(scriptPath string) {
		scriptPath = filepath.Clean(scriptPath)
		if visited[scriptPath] {
			return
		}
		visited[scriptPath] = true

		src, err := os.ReadFile(scriptPath)
		if err != nil {
			return
		}
		content := stripComments(string(src))

		for _, m := range projectDepRe.FindAllStringSubmatch(content, -1) {
			add(m[1], normalizeModuleName(m[2]))
		}
		for _, m := range accessorDepRe.FindAllStringSubmatch(content, -1) {
			target := accessorMap[m[2]]
			if target == "" {
				// include 一覧に見つからなければ素朴に kebab-case へ戻す
				segs := strings.Split(m[2], ".")
				for i, seg := range segs {
					segs[i] = kebabize(seg)
				}
				target = ":" + strings.Join(segs, ":")
			}
			add(m[1], target)
		}
		for _, line := range applyFromRe.FindAllString(content, -1) {
			if p := resolveApplyFrom(line, root, moduleDir); p != "" {
				scan(p)
			}
		}
	}
	scan(buildFile)

	sort.Slice(deps, func(i, j int) bool {
		if deps[i].Target != deps[j].Target {
			return deps[i].Target < deps[j].Target
		}
		return deps[i].Kind < deps[j].Kind
	})
	return deps
}

// resolveApplyFrom は apply from 行からスクリプトパスを解決する。
// rootProject.file / ${rootDir} / $rootDir はルート基準、素の相対パスはモジュールディレクトリ基準。
func resolveApplyFrom(line, root, moduleDir string) string {
	m := quotedRe.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	path := m[1]

	switch {
	case strings.Contains(path, "${rootDir}"):
		return filepath.Join(root, strings.Replace(path, "${rootDir}", "", 1))
	case strings.Contains(path, "$rootDir"):
		return filepath.Join(root, strings.Replace(path, "$rootDir", "", 1))
	case strings.Contains(path, "${rootProject.projectDir}"):
		return filepath.Join(root, strings.Replace(path, "${rootProject.projectDir}", "", 1))
	case strings.Contains(path, "$"):
		return "" // 解決できない変数参照
	case strings.Contains(line, "rootProject") || strings.Contains(line, "rootDir"):
		return filepath.Join(root, path)
	default:
		return filepath.Join(moduleDir, path)
	}
}

func kebabize(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r + ('a' - 'A'))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
