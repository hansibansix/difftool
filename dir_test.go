package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setupDirs(t *testing.T) (string, string) {
	t.Helper()
	l, r := t.TempDir(), t.TempDir()
	writeTestFile(t, filepath.Join(l, "same.txt"), "x\n")
	writeTestFile(t, filepath.Join(r, "same.txt"), "x\n")
	writeTestFile(t, filepath.Join(l, "mod.txt"), "a\n")
	writeTestFile(t, filepath.Join(r, "mod.txt"), "b\n")
	writeTestFile(t, filepath.Join(l, "onlyl.txt"), "l\n")
	writeTestFile(t, filepath.Join(r, "sub", "onlyr.txt"), "r\n")
	return l, r
}

func TestDirScan(t *testing.T) {
	l, r := setupDirs(t)
	d, err := newDirModel(l, r)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]dirStatus{
		"mod.txt":                         stModified,
		"onlyl.txt":                       stOnlyLeft,
		filepath.Join("sub", "onlyr.txt"): stOnlyRight,
		"same.txt":                        stSame,
	}
	if len(d.entries) != len(want) {
		t.Fatalf("got %d entries: %+v", len(d.entries), d.entries)
	}
	for _, e := range d.entries {
		if want[e.rel] != e.status {
			t.Errorf("%s: got status %v want %v", e.rel, e.status, want[e.rel])
		}
	}
	if n := fileRowCount(d); n != 3 {
		t.Fatalf("visible list should hide same files, got %d", n)
	}
	d.showAll = true
	d.rebuildList()
	if n := fileRowCount(d); n != 4 {
		t.Fatalf("show all should list 4, got %d", n)
	}
	var headers []string
	for _, r := range d.rows {
		if r.header != "" {
			headers = append(headers, r.header)
		}
	}
	if len(headers) != 2 || headers[0] != "./" || headers[1] != "sub/" {
		t.Fatalf("unexpected group headers %v", headers)
	}
}

func fileRowCount(d *dirModel) int {
	n := 0
	for _, r := range d.rows {
		if r.header == "" {
			n++
		}
	}
	return n
}

func selectRel(t *testing.T, d *dirModel, rel string) {
	t.Helper()
	for i, r := range d.rows {
		if r.header == "" && d.entries[r.ei].rel == rel {
			d.sel = i
			return
		}
	}
	t.Fatalf("entry %q not in list", rel)
}

func TestDirCopyEntry(t *testing.T) {
	l, r := setupDirs(t)
	d, err := newDirModel(l, r)
	if err != nil {
		t.Fatal(err)
	}
	selectRel(t, d, "onlyl.txt")
	d.copyEntry(true)
	data, err := os.ReadFile(filepath.Join(r, "onlyl.txt"))
	if err != nil || string(data) != "l\n" {
		t.Fatalf("copy failed: %v %q", err, data)
	}
	if n := fileRowCount(d); n != 3 {
		t.Fatalf("copied entry should stay visible as applied, got %d rows", n)
	}
	if d.selected().status != stApplied {
		t.Fatalf("copied entry should be stApplied, got %v", d.selected().status)
	}

	// copy ◀ with the file only existing on the right
	selectRel(t, d, filepath.Join("sub", "onlyr.txt"))
	d.copyEntry(false)
	if _, err := os.Stat(filepath.Join(l, "sub", "onlyr.txt")); err != nil {
		t.Fatalf("copy ◀ should create file on left: %v", err)
	}
}

func TestFilesEqual(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	writeTestFile(t, a, "same")
	writeTestFile(t, b, "same")
	if !filesEqual(a, b) {
		t.Fatal("equal files reported different")
	}
	writeTestFile(t, b, "diff")
	if filesEqual(a, b) {
		t.Fatal("different files reported equal")
	}
}

func TestIsBinary(t *testing.T) {
	dir := t.TempDir()
	bin, txt := filepath.Join(dir, "bin"), filepath.Join(dir, "txt")
	writeTestFile(t, bin, "ab\x00cd")
	writeTestFile(t, txt, "hello\n")
	if !isBinary(bin) || isBinary(txt) {
		t.Fatal("binary detection wrong")
	}
}

// a file synced via the embedded file view keeps a visible "applied" status
func TestRefreshSelectedMarksApplied(t *testing.T) {
	l, r := setupDirs(t)
	d, err := newDirModel(l, r)
	if err != nil {
		t.Fatal(err)
	}
	selectRel(t, d, "mod.txt")
	writeTestFile(t, filepath.Join(r, "mod.txt"), "a\n") // now equal on disk
	d.refreshSelected()
	if d.selected() == nil || d.selected().status != stApplied {
		t.Fatalf("expected stApplied, got %+v", d.selected())
	}
}
