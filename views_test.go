package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func seq(from, to int) []string {
	var out []string
	for i := from; i < to; i++ {
		out = append(out, "line "+string(rune('a'+i%26))+string(rune('0'+i/26)))
	}
	return out
}

func TestFoldRows(t *testing.T) {
	cfg.Fold = true
	defer func() { cfg.Fold = false }()
	left := seq(0, 30)
	right := append(append(append([]string{}, left[:10]...), "CHANGED"), left[11:]...)
	m := testModel(left, right)
	// leading equal chunk 0..10: fold 0..7, keep 7..10; trailing 11..30: keep 11..14, fold 14..30
	var folds []int
	for _, r := range m.rows {
		if r.fold > 0 {
			folds = append(folds, r.fold)
		}
	}
	if len(folds) != 2 || folds[0] != 7 || folds[1] != 16 {
		t.Fatalf("folds = %v", folds)
	}
	if len(m.rows) != 2+3+1+3 {
		t.Fatalf("rows = %d", len(m.rows))
	}
	if len(m.nav) != 1 || m.nav[0].row != 4 {
		t.Fatalf("nav = %+v", m.nav)
	}
	// applying keeps the region and its context visible
	m.apply(true)
	for _, r := range m.rows {
		if r.l == 10 && r.fold == 0 {
			goto ok
		}
	}
	t.Fatal("applied line must stay visible")
ok:
	// clicking a fold expands that chunk only
	m.unfold(m.rows[0].ci)
	if m.rows[0].fold != 0 || len(m.rows) < 30 {
		t.Fatalf("first chunk should be expanded: %+v", m.rows[:3])
	}
	// searching disables folding so matches inside folds are reachable
	m.unfolded = nil
	m.search = "line b1"
	m.recompute()
	if len(m.matches) != 1 || len(m.rows) != 30 {
		t.Fatalf("search must unfold: matches=%v rows=%d", m.matches, len(m.rows))
	}
	m.clearSearch()
	if len(m.rows) == 30 {
		t.Fatal("clearing the search must fold again")
	}
}

func TestFoldLineWidth(t *testing.T) {
	initStyles(themes["ansi"])
	if w := lipgloss.Width(foldLine(row{l: 4, r: 6, fold: 12}, 60)); w != 60 {
		t.Fatalf("width %d", w)
	}
}

func TestUnifiedRows(t *testing.T) {
	cfg.Unified = true
	defer func() { cfg.Unified = false }()
	m := testModel([]string{"a", "b", "c"}, []string{"a", "X", "Y", "c"})
	// a | -b | +X | +Y | c
	if len(m.rows) != 5 || m.rows[1].l != 1 || m.rows[1].r != -1 || m.rows[2].l != -1 || m.rows[2].r != 1 {
		t.Fatalf("rows = %+v", m.rows)
	}
	first, last := m.chunkRows()
	if first != 1 || last != 3 {
		t.Fatalf("chunkRows = %d,%d", first, last)
	}
	m.w, m.h = 60, 8
	initStyles(themes["ansi"])
	for _, l := range strings.Split(m.view(true), "\n") {
		if w := lipgloss.Width(l); w != 60 {
			t.Fatalf("line width %d: %q", w, l)
		}
	}
	m.apply(true)
	if len(m.rows) != 3 || len(m.nav) != 1 || m.nav[0].ai != 0 {
		t.Fatalf("after apply: rows=%d nav=%+v", len(m.rows), m.nav)
	}
}

func TestIgnoreBlankAndRegex(t *testing.T) {
	defer func() { cfg.IgnoreBlank, cfg.IgnoreRegex = false, "" }()
	left := []string{"a", "// v1", "b", "c", "d"}
	right := []string{"a", "// v2", "b", "", "", "c", "D"}
	m := testModel(left, right)
	if len(m.nav) != 3 {
		t.Fatalf("nav = %d", len(m.nav))
	}
	cfg.IgnoreBlank = true
	m.recompute()
	if len(m.nav) != 2 || m.skippedCount() != 1 {
		t.Fatalf("blank change must be ignored: nav=%d skipped=%d", len(m.nav), m.skippedCount())
	}
	cfg.IgnoreRegex = `^//`
	m.recompute()
	if len(m.nav) != 1 || m.nav[0].ci != 5 {
		t.Fatalf("comment change must be ignored: %+v", m.nav)
	}
	m.applyAll(true)
	if strings.Join(m.right, ",") != "a,// v2,b,,,c,d" {
		t.Fatalf("applyAll must skip ignored chunks: %v", m.right)
	}
	cfg.IgnoreRegex = `(`
	if ignoreRe() != nil || regexSummary() != "invalid: (" {
		t.Fatalf("invalid regex: %v %q", ignoreRe(), regexSummary())
	}
}

func TestUnifiedDiff(t *testing.T) {
	left := append(append([]string{}, seq(0, 10)...), "tail")
	right := append([]string{}, left...)
	right[2] = "TWO"
	right = append(right[:8], right[9:]...) // delete line 8
	m := testModel(left, right)
	m.rightPath = "/x/file.txt"
	patch, n := m.unifiedDiff(-1, 3)
	if n != 1 { // 5 equal lines between the changes < 2*ctx: one hunk
		t.Fatalf("hunks = %d\n%s", n, patch)
	}
	want := "--- a//x/file.txt\n+++ b//x/file.txt\n@@ -1,11 +1,10 @@\n line a0\n line b0\n-line c0\n+TWO\n line d0\n line e0\n line f0\n line g0\n line h0\n-line i0\n line j0\n tail\n"
	if patch != want {
		t.Fatalf("patch:\n%s\nwant:\n%s", patch, want)
	}
	_, n = m.unifiedDiff(-1, 1)
	if n != 2 {
		t.Fatalf("with 1 context line the changes must split: %d", n)
	}
	m.rightNL = false
	patch, _ = m.unifiedDiff(-1, 3)
	if !strings.HasSuffix(patch, " tail\n") || strings.Contains(patch, "No newline") {
		t.Fatalf("context lines come from the left side:\n%s", patch)
	}
	// single chunk export
	patch, n = m.unifiedDiff(m.nav[1].ci, 3)
	if n != 1 || strings.Contains(patch, "TWO") {
		t.Fatalf("only the second hunk:\n%s", patch)
	}
}

func TestUnifiedDiffNoNewline(t *testing.T) {
	m := testModel([]string{"a"}, []string{"b"})
	m.leftNL, m.rightNL = false, true
	patch, _ := m.unifiedDiff(-1, 3)
	if !strings.Contains(patch, "-a\n\\ No newline at end of file\n+b\n") {
		t.Fatalf("patch:\n%s", patch)
	}
	if !strings.Contains(patch, "@@ -1,1 +1,1 @@") {
		t.Fatalf("header:\n%s", patch)
	}
	m = testModel([]string{}, []string{"a", "b"})
	patch, _ = m.unifiedDiff(-1, 3)
	if !strings.Contains(patch, "@@ -0,0 +1,2 @@") {
		t.Fatalf("empty left range:\n%s", patch)
	}
}

func TestEditorCmd(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "nvim -u NONE")
	c := editorCmd("/tmp/f.txt", 12)
	if got := strings.Join(c.Args, " "); got != "nvim -u NONE +12 /tmp/f.txt" {
		t.Fatalf("args = %q", got)
	}
	t.Setenv("EDITOR", "code --wait")
	c = editorCmd("/tmp/f.txt", 12)
	if got := strings.Join(c.Args, " "); got != "code --wait /tmp/f.txt" {
		t.Fatalf("args = %q", got)
	}
}

func TestEditRefusedWhileDirty(t *testing.T) {
	m := testModel([]string{"a"}, []string{"b"})
	m.apply(true)
	if cmd := m.editIn(true); cmd != nil || !strings.Contains(m.status, "save") {
		t.Fatalf("dirty edit must be refused: %q", m.status)
	}
	m.roRight = true
	m.undoLast()
	if cmd := m.editIn(true); cmd != nil || !strings.Contains(m.status, "read-only") {
		t.Fatalf("read-only edit must be refused: %q", m.status)
	}
}
