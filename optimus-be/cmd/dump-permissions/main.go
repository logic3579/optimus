// Command dump-permissions emits the registered permission catalogue as a
// markdown document on stdout. It is invoked by `make dump-perms` to keep
// `docs/permissions.md` in sync with `internal/infra/permissions/codes.go`.
// CI runs `make perm-check` to diff the regenerated output against the
// committed file and fails if they drift.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"optimus-be/internal/infra/permissions"
)

func main() {
	out := strings.Builder{}
	out.WriteString("# P0 Permissions Registry\n\n")
	out.WriteString("Auto-generated from `optimus-be/internal/infra/permissions/codes.go`. ")
	out.WriteString("Run `make dump-perms` to refresh. CI fails if this is stale.\n\n")

	byCat := map[string][]permissions.Permission{}
	cats := []string{}
	for _, p := range permissions.All {
		if _, ok := byCat[p.Category]; !ok {
			cats = append(cats, p.Category)
		}
		byCat[p.Category] = append(byCat[p.Category], p)
	}
	sort.Strings(cats)
	for _, c := range cats {
		out.WriteString("## " + c + "\n\n")
		out.WriteString("| Code | Name (i18n) | Description |\n|---|---|---|\n")
		ps := byCat[c]
		sort.Slice(ps, func(i, j int) bool { return ps[i].Code < ps[j].Code })
		for _, p := range ps {
			out.WriteString("| `" + p.Code + "` | `" + p.Name + "` | " + p.Description + " |\n")
		}
		out.WriteString("\n")
	}
	if _, err := os.Stdout.WriteString(out.String()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
