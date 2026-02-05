package migrations

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
)

//go:embed *.sql
var embedded embed.FS

type Migration struct {
	ID  string
	SQL string
}

func All() ([]Migration, error) {
	names, err := fs.Glob(embedded, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("glob migrations: %w", err)
	}
	sort.Strings(names)

	out := make([]Migration, 0, len(names))
	for _, name := range names {
		b, err := fs.ReadFile(embedded, name)
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", name, err)
		}
		out = append(out, Migration{
			ID:  name,
			SQL: string(b),
		})
	}
	return out, nil
}

