package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kr9ly/skeleton/skeleton"
)

func TextModules(g *skeleton.ModuleGraph) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s (%s, %d modules)\n\n", g.Root, g.System, len(g.Modules))

	for _, m := range g.Modules {
		b.WriteString(m.Name)
		b.WriteString("\n")
		for _, d := range m.Deps {
			fmt.Fprintf(&b, "  -> %s (%s)\n", d.Target, d.Kind)
		}
	}

	// 逆依存: 被依存数の多い順（ディレクトリモードの deps と同じ流儀）
	usersOf := make(map[string][]string)
	for _, m := range g.Modules {
		seen := make(map[string]bool)
		for _, d := range m.Deps {
			if !seen[d.Target] {
				seen[d.Target] = true
				usersOf[d.Target] = append(usersOf[d.Target], m.Name)
			}
		}
	}
	if len(usersOf) > 0 {
		b.WriteString("\n## reverse deps\n")
		type rev struct {
			target string
			users  []string
		}
		var revs []rev
		for target, users := range usersOf {
			sort.Strings(users)
			revs = append(revs, rev{target, users})
		}
		sort.Slice(revs, func(i, j int) bool {
			if len(revs[i].users) != len(revs[j].users) {
				return len(revs[i].users) > len(revs[j].users)
			}
			return revs[i].target < revs[j].target
		})
		for _, r := range revs {
			fmt.Fprintf(&b, "%s <- %s\n", r.target, strings.Join(r.users, ", "))
		}
	}

	return b.String()
}
