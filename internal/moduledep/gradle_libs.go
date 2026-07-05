package moduledep

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/kr9ly/skeleton/skeleton"
)

var (
	// implementation("io.ktor:ktor-client-core:2.3.0") / api 'com.x:y:1.0'
	// platform()/enforcedPlatform() ラッパー越しの宣言にも対応
	coordDepRe = regexp.MustCompile(`(\w+)\s*\(?\s*(?:(?:enforcedPlatform|platform)\s*\(\s*)?["']([^"'\s]+:[^"'\s]+)["']`)
	// implementation(libs.moshi.kotlin) — version catalog accessor。
	// 設定名の直後に空白か括弧を必須にする（"korlibs.time" 等の文字列内誤マッチを防ぐ）
	catalogDepRe = regexp.MustCompile(`(\w+)(?:\s+|\s*\(\s*)(?:(?:enforcedPlatform|platform)\s*\(\s*)?libs\.([A-Za-z0-9_.]+)`)
	// "group:artifact" または "group:artifact:version" の形だけをライブラリ座標とみなす
	// （URL 等の誤検出を除外。version には $var / ${var} / 範囲表記を許す）
	coordinateRe = regexp.MustCompile(`^[\w.\-]+:[\w.\-]+(?::[\w.\-+${}\[\](),!]+)?$`)
)

func (g *gradleProvider) ExtractLibs(root string) (*skeleton.LibsReport, error) {
	refs, err := g.moduleRefs(root)
	if err != nil {
		return nil, err
	}

	catalog := loadVersionCatalog(filepath.Join(root, "gradle", "libs.versions.toml"))

	var scopes []skeleton.LibScope
	addScope := func(name, dir string) {
		if libs := parseLibs(root, dir, catalog); len(libs) > 0 {
			scopes = append(scopes, skeleton.LibScope{Name: name, Libs: libs})
		}
	}

	for _, ref := range refs {
		addScope(ref.name, filepath.Join(root, ref.dir))
	}
	// ルート（モジュールなしの単一プロジェクトはこれだけになる）
	addScope(":", root)

	sort.Slice(scopes, func(i, j int) bool { return scopes[i].Name < scopes[j].Name })

	return &skeleton.LibsReport{Root: root, System: "gradle", Scopes: scopes}, nil
}

// parseLibs は moduleDir の build.gradle(.kts) から外部ライブラリ宣言を抽出する
func parseLibs(root, moduleDir string, catalog *versionCatalog) []skeleton.Lib {
	seen := make(map[skeleton.Lib]bool)
	var libs []skeleton.Lib
	add := func(l skeleton.Lib) {
		if !seen[l] {
			seen[l] = true
			libs = append(libs, l)
		}
	}

	scanBuildScripts(root, moduleDir, func(content string) {
		for _, m := range coordDepRe.FindAllStringSubmatch(content, -1) {
			kind, coord := m[1], m[2]
			if !coordinateRe.MatchString(coord) {
				continue
			}
			lib := skeleton.Lib{Kind: kind}
			if i := strings.LastIndex(coord, ":"); strings.Count(coord, ":") == 2 {
				lib.Coordinate, lib.Version = coord[:i], coord[i+1:]
			} else {
				lib.Coordinate = coord
			}
			add(lib)
		}
		for _, m := range catalogDepRe.FindAllStringSubmatch(content, -1) {
			kind, accessor := m[1], normalizeAccessor(m[2])
			switch {
			case strings.HasPrefix(accessor, "versions.") || strings.HasPrefix(accessor, "plugins."):
				// バージョン参照・プラグインはライブラリではない
			case strings.HasPrefix(accessor, "bundles."):
				for _, l := range catalog.bundle(strings.TrimPrefix(accessor, "bundles.")) {
					l.Kind = kind
					add(l)
				}
			default:
				l := catalog.library(accessor)
				l.Kind = kind
				add(l)
			}
		}
	})

	sort.Slice(libs, func(i, j int) bool {
		if libs[i].Coordinate != libs[j].Coordinate {
			return libs[i].Coordinate < libs[j].Coordinate
		}
		if libs[i].Version != libs[j].Version {
			return libs[i].Version < libs[j].Version
		}
		return libs[i].Kind < libs[j].Kind
	})
	return libs
}

// versionCatalog は gradle/libs.versions.toml の逆引き表。
// キーは accessor 表記（kebab/snake → ドット区切り）に正規化して持つ。
type versionCatalog struct {
	libraries map[string]skeleton.Lib // "moshi.kotlin" → {Coordinate, Version}
	bundles   map[string][]string     // "network" → ["okhttp", "retrofit.core"]
}

// library は accessor に対応する座標を返す。カタログに無ければ宣言表記のまま返す
func (c *versionCatalog) library(accessor string) skeleton.Lib {
	if c != nil {
		if l, ok := c.libraries[accessor]; ok {
			return l
		}
	}
	return skeleton.Lib{Coordinate: "libs." + accessor}
}

func (c *versionCatalog) bundle(name string) []skeleton.Lib {
	if c == nil || len(c.bundles[name]) == 0 {
		return []skeleton.Lib{{Coordinate: "libs.bundles." + name}}
	}
	var libs []skeleton.Lib
	for _, key := range c.bundles[name] {
		libs = append(libs, c.library(key))
	}
	return libs
}

// normalizeAccessor はカタログキー・accessor の "-" "_" を "." に揃える
func normalizeAccessor(s string) string {
	s = strings.ReplaceAll(s, "-", ".")
	return strings.ReplaceAll(s, "_", ".")
}

// TOML はダブルクォート（basic string）とシングルクォート（literal string）の両方を許す
var (
	tomlSectionRe = regexp.MustCompile(`^\[([\w.\-]+)\]`)
	tomlKeyRe     = regexp.MustCompile(`^([\w.\-]+)\s*=\s*(.+)$`)
	tomlStrRe     = regexp.MustCompile(`^["']([^"']*)["']`)
	tomlModuleRe  = regexp.MustCompile(`module\s*=\s*["']([^"']+)["']`)
	tomlGroupRe   = regexp.MustCompile(`group\s*=\s*["']([^"']+)["']`)
	tomlNameRe    = regexp.MustCompile(`name\s*=\s*["']([^"']+)["']`)
	tomlVerRefRe  = regexp.MustCompile(`version\.ref\s*=\s*["']([^"']+)["']`)
	tomlVerRe     = regexp.MustCompile(`version\s*=\s*["']([^"']+)["']`)
	tomlArrayRe   = regexp.MustCompile(`["']([^"']+)["']`)
)

// loadVersionCatalog は libs.versions.toml を行ベースで読む簡易パーサー。
// [versions] / [libraries] / [bundles] のみ解釈し、ファイルが無ければ nil を返す
func loadVersionCatalog(path string) *versionCatalog {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	versions := make(map[string]string)
	type rawLib struct {
		key, coordinate, version, versionRef string
	}
	var rawLibs []rawLib
	bundles := make(map[string][]string)

	section := ""
	var bundleKey string // 複数行配列の継続中バンドル名
	var bundleBuf string // 配列本体のバッファ

	for _, line := range strings.Split(string(src), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if m := tomlSectionRe.FindStringSubmatch(line); m != nil {
			section = m[1]
			bundleKey = ""
			continue
		}

		// バンドル配列の複数行継続
		if bundleKey != "" {
			bundleBuf += line
			if strings.Contains(line, "]") {
				for _, s := range tomlArrayRe.FindAllStringSubmatch(bundleBuf, -1) {
					key := normalizeAccessor(bundleKey)
					bundles[key] = append(bundles[key], normalizeAccessor(s[1]))
				}
				bundleKey = ""
			}
			continue
		}

		m := tomlKeyRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key, value := m[1], m[2]

		switch section {
		case "versions":
			if s := tomlStrRe.FindStringSubmatch(value); s != nil {
				versions[key] = s[1]
			}
		case "libraries":
			raw := rawLib{key: normalizeAccessor(key)}
			if s := tomlStrRe.FindStringSubmatch(value); s != nil {
				// key = "group:artifact:version"
				coord := s[1]
				if i := strings.LastIndex(coord, ":"); strings.Count(coord, ":") == 2 {
					raw.coordinate, raw.version = coord[:i], coord[i+1:]
				} else {
					raw.coordinate = coord
				}
			} else {
				// key = { module = "g:a", version.ref = "x" } / { group = ..., name = ..., version = ... }
				if mm := tomlModuleRe.FindStringSubmatch(value); mm != nil {
					raw.coordinate = mm[1]
				} else if g, n := tomlGroupRe.FindStringSubmatch(value), tomlNameRe.FindStringSubmatch(value); g != nil && n != nil {
					raw.coordinate = g[1] + ":" + n[1]
				}
				if vr := tomlVerRefRe.FindStringSubmatch(value); vr != nil {
					raw.versionRef = vr[1]
				} else if v := tomlVerRe.FindStringSubmatch(value); v != nil {
					raw.version = v[1]
				}
			}
			if raw.coordinate != "" {
				rawLibs = append(rawLibs, raw)
			}
		case "bundles":
			if strings.Contains(value, "]") {
				for _, s := range tomlArrayRe.FindAllStringSubmatch(value, -1) {
					bundles[normalizeAccessor(key)] = append(bundles[normalizeAccessor(key)], normalizeAccessor(s[1]))
				}
			} else {
				bundleKey = key
				bundleBuf = value
			}
		}
	}

	catalog := &versionCatalog{libraries: make(map[string]skeleton.Lib), bundles: bundles}
	for _, raw := range rawLibs {
		version := raw.version
		if raw.versionRef != "" {
			version = versions[raw.versionRef]
		}
		catalog.libraries[raw.key] = skeleton.Lib{Coordinate: raw.coordinate, Version: version}
	}
	return catalog
}
