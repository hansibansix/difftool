package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func mergeFixture(t *testing.T, local, base, remote string) (*model, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	lp, bp, rp, mp := filepath.Join(dir, "local"), filepath.Join(dir, "base"), filepath.Join(dir, "remote"), filepath.Join(dir, "merged")
	writeTestFile(t, lp, local)
	writeTestFile(t, bp, base)
	writeTestFile(t, rp, remote)
	m, err := newMergeModel(lp, bp, rp, mp)
	if err != nil {
		t.Fatal(err)
	}
	return m, mp
}

func TestMergeCleanAutoResolves(t *testing.T) {
	base := "1\n2\n3\n4\n5\n6\n7\n"
	m, mp := mergeFixture(t, strings.Replace(base, "2", "L", 1), base, strings.Replace(base, "6", "R", 1))
	if m.conflicts() != 0 {
		t.Fatalf("conflicts = %d", m.conflicts())
	}
	if !reflect.DeepEqual(m.right, []string{"1", "L", "3", "4", "5", "R", "7"}) {
		t.Fatalf("merged = %v", m.right)
	}
	if !m.dirty() {
		t.Fatal("result is unsaved until written")
	}
	m.save()
	data, _ := os.ReadFile(mp)
	if string(data) != "1\nL\n3\n4\n5\nR\n7\n" {
		t.Fatalf("written %q", data)
	}
}

func TestMergeConflictResolveWithRemote(t *testing.T) {
	m, _ := mergeFixture(t, "a\nX\nc\n", "a\nb\nc\n", "a\nY\nc\n")
	if m.conflicts() != 1 || !strings.HasPrefix(m.right[1], "<<<<<<< LOCAL") {
		t.Fatalf("expected one conflict block, got %v", m.right)
	}
	if m.leftName != "LOCAL" || m.left[1] != "X" {
		t.Fatalf("left should start on LOCAL: %s %v", m.leftName, m.left)
	}
	// take REMOTE: switch the left side, apply its hunk onto the marker block
	m.switchSide(2)
	if m.leftName != "REMOTE" || m.left[1] != "Y" || len(m.nav) != 1 {
		t.Fatalf("switch: %s %v nav=%v", m.leftName, m.left, m.nav)
	}
	m.apply(true)
	if m.conflicts() != 0 || !reflect.DeepEqual(m.right, []string{"a", "Y", "c"}) {
		t.Fatalf("resolved = %v (%d conflicts)", m.right, m.conflicts())
	}
	// undo the apply, then undo the side switch: back on LOCAL with markers
	m.undoLast()
	if m.conflicts() != 1 || m.merge.idx != 2 {
		t.Fatalf("undo apply: conflicts=%d side=%d", m.conflicts(), m.merge.idx)
	}
	m.undoLast()
	if m.merge.idx != 0 || m.leftName != "LOCAL" || m.left[1] != "X" {
		t.Fatalf("undo switch: side=%d name=%s left=%v", m.merge.idx, m.leftName, m.left)
	}
	// left side is never writable
	m.apply(false)
	if m.left[1] != "X" || !strings.Contains(m.status, "read-only") {
		t.Fatalf("apply ◀ must be refused: %v %q", m.left, m.status)
	}
}
