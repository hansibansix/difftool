package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteOneSidedAndUndo(t *testing.T) {
	l, r := setupDirs(t)
	d, err := newDirModel(l, r)
	if err != nil {
		t.Fatal(err)
	}
	// rightonly-style entry: sub/onlyr.txt exists only on the right;
	// applying left → right means deleting it there, after confirmation
	selectRel(t, d, filepath.Join("sub", "onlyr.txt"))
	d.copyEntry(true)
	if d.pendingDelete == "" {
		t.Fatalf("expected a delete prompt, status %q", d.status)
	}
	d.deletePending()
	if _, err := os.Stat(filepath.Join(r, "sub", "onlyr.txt")); err == nil {
		t.Fatal("file must be deleted")
	}
	if d.selected().status != stDeleted || d.compare(d.selected().rel) != stDeleted {
		t.Fatalf("status %v", d.selected().status)
	}
	d.undoCopy()
	data, err := os.ReadFile(filepath.Join(r, "sub", "onlyr.txt"))
	if err != nil || string(data) != "r\n" {
		t.Fatalf("undo must restore the file: %v %q", err, data)
	}
	if d.selected().status != stOnlyRight {
		t.Fatalf("status after undo %v", d.selected().status)
	}
}

func TestSyncAllAndUndo(t *testing.T) {
	l, r := setupDirs(t)
	d, err := newDirModel(l, r)
	if err != nil {
		t.Fatal(err)
	}
	copies, deletes := d.syncPlan(true)
	if copies != 2 || deletes != 1 { // mod.txt + onlyl.txt copied, sub/onlyr.txt deleted
		t.Fatalf("plan = %d copies, %d deletes", copies, deletes)
	}
	d.syncAll(true)
	if got, _ := os.ReadFile(filepath.Join(r, "mod.txt")); string(got) != "a\n" {
		t.Fatalf("mod.txt not synced: %q", got)
	}
	if _, err := os.Stat(filepath.Join(r, "onlyl.txt")); err != nil {
		t.Fatal("onlyl.txt must be copied")
	}
	if _, err := os.Stat(filepath.Join(r, "sub", "onlyr.txt")); err == nil {
		t.Fatal("sub/onlyr.txt must be deleted")
	}
	c2, d2 := d.syncPlan(true)
	if c2+d2 != 0 {
		t.Fatalf("after sync nothing should be left: %d %d", c2, d2)
	}
	// one undo reverts the whole batch
	d.undoCopy()
	if got, _ := os.ReadFile(filepath.Join(r, "mod.txt")); string(got) != "b\n" {
		t.Fatalf("undo mod.txt: %q", got)
	}
	if _, err := os.Stat(filepath.Join(r, "onlyl.txt")); err == nil {
		t.Fatal("undo must remove the copied onlyl.txt")
	}
	if got, _ := os.ReadFile(filepath.Join(r, "sub", "onlyr.txt")); string(got) != "r\n" {
		t.Fatalf("undo must restore sub/onlyr.txt: %q", got)
	}
	if len(d.undo) != 0 {
		t.Fatalf("undo stack should be empty, has %d", len(d.undo))
	}
}

func TestSyncRefusedOnReadOnlyTarget(t *testing.T) {
	l, r := setupDirs(t)
	d, err := newDirModel(l, r)
	if err != nil {
		t.Fatal(err)
	}
	d.roLeft = true
	d.askSync(false)
	if d.syncStep != 0 || d.status != "target side is read-only (git ref)" {
		t.Fatalf("step %d status %q", d.syncStep, d.status)
	}
	d.askSync(true)
	if d.syncStep != 2 {
		t.Fatalf("expected confirmation step, got %d (%q)", d.syncStep, d.status)
	}
}
