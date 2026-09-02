package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// Tree-level file operations. Each user action bumps d.batch so that every
// file touched by it undoes together.

// copyOp copies e from one side to the other, recording the overwritten
// content for undo.
func (d *dirModel) copyOp(e *dirEntry, toRight bool) error {
	src, dst := filepath.Join(d.leftRoot, e.rel), filepath.Join(d.rightRoot, e.rel)
	if !toRight {
		src, dst = dst, src
	}
	u := copyUndo{rel: e.rel, dst: dst, status: e.status, batch: d.batch}
	if info, err := os.Stat(dst); err == nil {
		u.mode = info.Mode().Perm()
		if u.prev, err = os.ReadFile(dst); err != nil {
			return err
		}
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	d.undo = append(d.undo, u)
	e.status, e.hasStat = stApplied, false
	return nil
}

// deleteOp removes e on the target side (the side that has it while the
// other does not), keeping the content for undo.
func (d *dirModel) deleteOp(e *dirEntry, toRight bool) error {
	dst := filepath.Join(d.leftRoot, e.rel)
	if toRight {
		dst = filepath.Join(d.rightRoot, e.rel)
	}
	info, err := os.Stat(dst)
	if err != nil {
		return err
	}
	prev, err := os.ReadFile(dst)
	if err != nil {
		return err
	}
	if err := os.Remove(dst); err != nil {
		return err
	}
	d.undo = append(d.undo, copyUndo{rel: e.rel, dst: dst, prev: prev, mode: info.Mode().Perm(), status: e.status, batch: d.batch})
	e.status, e.hasStat = stDeleted, false
	return nil
}

// deletePending performs the deletion the user confirmed with y.
func (d *dirModel) deletePending() {
	dst := d.pendingDelete
	d.pendingDelete = ""
	e := d.selected()
	if e == nil {
		return
	}
	d.batch++
	if err := d.deleteOp(e, dst == filepath.Join(d.rightRoot, e.rel)); err != nil {
		d.status = "error: " + err.Error()
		return
	}
	d.status = "✕ deleted " + e.rel
	d.rebuildList()
}

// syncPlan counts what syncing the listed files in one direction would do.
func (d *dirModel) syncPlan(toRight bool) (copies, deletes int) {
	for _, r := range d.rows {
		if r.header != "" {
			continue
		}
		switch d.entries[r.ei].status {
		case stModified:
			copies++
		case stOnlyLeft:
			if toRight {
				copies++
			} else {
				deletes++
			}
		case stOnlyRight:
			if toRight {
				deletes++
			} else {
				copies++
			}
		}
	}
	return copies, deletes
}

// askSync validates the direction and asks for confirmation with a summary.
func (d *dirModel) askSync(toRight bool) {
	d.syncStep = 0
	if (toRight && d.roRight) || (!toRight && d.roLeft) {
		d.status = "target side is read-only (git ref)"
		return
	}
	copies, deletes := d.syncPlan(toRight)
	if copies+deletes == 0 {
		d.status = "nothing to sync"
		return
	}
	d.syncStep, d.syncToRight = 2, toRight
	d.status = fmt.Sprintf("sync %s %d files (%d copy, %d delete)? y/n", arrowOf(toRight), copies+deletes, copies, deletes)
}

// syncAll makes the target side match the source side for every listed
// file: modified and one-sided-on-source files are copied, files only on
// the target side are deleted. One undo step.
func (d *dirModel) syncAll(toRight bool) {
	d.batch++
	var copies, deletes, errs int
	for _, r := range d.rows {
		if r.header != "" {
			continue
		}
		e := &d.entries[r.ei]
		var err error
		switch {
		case e.status == stModified,
			e.status == stOnlyLeft && toRight,
			e.status == stOnlyRight && !toRight:
			err = d.copyOp(e, toRight)
			copies++
		case e.status == stOnlyLeft || e.status == stOnlyRight:
			err = d.deleteOp(e, toRight)
			deletes++
		default:
			continue
		}
		if err != nil {
			errs++
		}
	}
	d.status = fmt.Sprintf("synced %s: %d copied, %d deleted", arrowOf(toRight), copies, deletes)
	if errs > 0 {
		d.status += fmt.Sprintf(", %d errors", errs)
	}
	d.rebuildList()
}
