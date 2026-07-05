package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kr9ly/skeleton/skeleton"
)

func TextLibs(r *skeleton.LibsReport) string {
	var b strings.Builder

	// coordinate → version → 使用スコープ集合
	byLib := make(map[string]map[string][]string)
	for _, s := range r.Scopes {
		seen := make(map[string]bool)
		for _, l := range s.Libs {
			key := l.Coordinate + "\x00" + l.Version
			if seen[key] {
				continue
			}
			seen[key] = true
			if byLib[l.Coordinate] == nil {
				byLib[l.Coordinate] = make(map[string][]string)
			}
			byLib[l.Coordinate][l.Version] = append(byLib[l.Coordinate][l.Version], s.Name)
		}
	}

	fmt.Fprintf(&b, "# %s (%s, %d libraries)\n\n", r.Root, r.System, len(byLib))

	type entry struct {
		coordinate string
		versions   map[string][]string
		userCount  int
	}
	var entries []entry
	for coord, versions := range byLib {
		users := make(map[string]bool)
		for _, scopes := range versions {
			for _, s := range scopes {
				users[s] = true
			}
		}
		entries = append(entries, entry{coord, versions, len(users)})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].userCount != entries[j].userCount {
			return entries[i].userCount > entries[j].userCount // 使用スコープの多い順
		}
		return entries[i].coordinate < entries[j].coordinate
	})

	// 単一スコープ（モジュールなしプロジェクト等）なら使用元リストは冗長なので省く
	singleScope := len(r.Scopes) == 1

	// 使用元が多い場合はリストの代わりに件数だけ出す（コンパクトさ優先）
	const listThreshold = 12
	userList := func(scopes []string) string {
		if len(scopes) > listThreshold {
			return fmt.Sprintf("(%d modules)", len(scopes))
		}
		return strings.Join(scopes, ", ")
	}

	for _, e := range entries {
		var versions []string
		for v := range e.versions {
			versions = append(versions, v)
		}
		sort.Strings(versions)

		if len(versions) == 1 {
			line := e.coordinate
			if versions[0] != "" {
				line += " " + versions[0]
			}
			if !singleScope {
				line += " <- " + userList(e.versions[versions[0]])
			}
			b.WriteString(line)
			b.WriteString("\n")
			continue
		}

		// バージョンが割れている場合はバージョンごとに使用元を並べる
		b.WriteString(e.coordinate)
		b.WriteString("\n")
		for _, v := range versions {
			label := v
			if label == "" {
				label = "(no version)"
			}
			if singleScope {
				fmt.Fprintf(&b, "  %s\n", label)
			} else {
				fmt.Fprintf(&b, "  %s <- %s\n", label, userList(e.versions[v]))
			}
		}
	}

	return b.String()
}
