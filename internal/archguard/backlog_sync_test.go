package archguard

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	readmeStatusLine = regexp.MustCompile("^\\- \\[[x ]\\] `([^`]+)` \\(`([0-9]+)\\/([0-9]+)` done\\)$")
	backlogItemLine  = regexp.MustCompile("^\\- \\[([x ])\\] [A-Z0-9\\-]+:")
)

func TestBacklogReadmeFileStatusMatchesBacklogFiles(t *testing.T) {
	readmePath := filepath.Join("..", "..", "docs", "dev", "backlog", "README.md")
	lines := readLines(t, readmePath)

	type statusSpec struct {
		Path        string
		DoneInSpec  int
		TotalInSpec int
	}
	specs := make([]statusSpec, 0, 8)
	for _, line := range lines {
		m := readmeStatusLine.FindStringSubmatch(strings.TrimSpace(line))
		if len(m) != 4 {
			continue
		}
		done, _ := strconv.Atoi(m[2])
		total, _ := strconv.Atoi(m[3])
		specs = append(specs, statusSpec{
			Path:        m[1],
			DoneInSpec:  done,
			TotalInSpec: total,
		})
	}
	if len(specs) == 0 {
		t.Fatalf("no backlog file status rows found in %s", readmePath)
	}

	for _, spec := range specs {
		backlogPath := filepath.Join("..", "..", spec.Path)
		done, total := countBacklogChecks(t, backlogPath)
		if done != spec.DoneInSpec || total != spec.TotalInSpec {
			t.Fatalf("file status mismatch for %s: readme=%d/%d actual=%d/%d", spec.Path, spec.DoneInSpec, spec.TotalInSpec, done, total)
		}
	}
}

func countBacklogChecks(t *testing.T, path string) (int, int) {
	t.Helper()
	lines := readLines(t, path)
	done := 0
	total := 0
	for _, line := range lines {
		m := backlogItemLine.FindStringSubmatch(strings.TrimSpace(line))
		if len(m) != 2 {
			continue
		}
		total++
		if m[1] == "x" {
			done++
		}
	}
	return done, total
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	out := make([]string, 0, 128)
	s := bufio.NewScanner(f)
	for s.Scan() {
		out = append(out, s.Text())
	}
	if err := s.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return out
}
