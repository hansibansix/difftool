package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// mergeSide is one of the three inputs of a 3-way merge (LOCAL, BASE,
// REMOTE); one of them is shown on the left of the diff view.
type mergeSide struct {
	name, path string
	lines      []string
	nl         bool
}

// mergeState is the git-mergetool state: the three inputs and which one the
// left pane currently shows. The right pane is the merge result.
type mergeState struct {
	sides [3]mergeSide
	idx   int
}

// newMergeModel builds the diff view for git mergetool: the right side is
// the merge result seeded by `git merge-file` (conflicts as marker blocks,
// unsaved until written to merged), the left side starts on LOCAL.
func newMergeModel(local, base, remote, merged string) (*model, error) {
	cmd := exec.Command("git", "merge-file", "-p",
		"-L", "LOCAL", "-L", "BASE", "-L", "REMOTE", local, base, remote)
	out, err := cmd.Output()
	if err != nil {
		// exit status = number of conflicts; the output is still the merge
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() < 0 {
			return nil, fmt.Errorf("git merge-file: %w", err)
		}
	}
	ms := &mergeState{}
	for i, p := range [3]string{local, base, remote} {
		lines, nl, err := readLines(p)
		if err != nil {
			return nil, err
		}
		ms.sides[i] = mergeSide{[3]string{"LOCAL", "BASE", "REMOTE"}[i], p, lines, nl}
	}
	right := []string{}
	if len(out) > 0 {
		right = strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	}
	m := &model{
		rightPath: merged, rightName: displayPath(merged),
		right: right, rightNL: true,
		roLeft: true, merge: ms,
	}
	m.setSide(0)
	m.recompute()
	m.status = fmt.Sprintf("%d conflicts · apply hunks onto the marker blocks to resolve", m.conflicts())
	return m, nil
}

// maskConflicts makes every line inside a conflict block unique, so the
// whole block diffs as one hunk instead of partially matching either side.
func maskConflicts(lines []string) []string {
	out := append([]string(nil), lines...)
	in := false
	for i, l := range out {
		if strings.HasPrefix(l, "<<<<<<<") {
			in = true
		}
		if in {
			out[i] = fmt.Sprintf("\x00conflict %d", i)
		}
		if strings.HasPrefix(l, ">>>>>>>") {
			in = false
		}
	}
	return out
}

// conflicts counts unresolved conflict blocks in the merge result.
func (m *model) conflicts() int {
	n := 0
	for _, l := range m.right {
		if strings.HasPrefix(l, "<<<<<<<") {
			n++
		}
	}
	return n
}

// switchSide shows another merge input on the left (undoable).
func (m *model) switchSide(i int) {
	if m.merge == nil || i == m.merge.idx {
		return
	}
	m.pushUndo()
	m.setSide(i)
	m.recompute()
	m.status = "left: " + m.merge.sides[i].name
}

// setSide swaps the left pane to merge input i. Applied markers refer to
// the previous side's lines, so they are dropped.
func (m *model) setSide(i int) {
	m.merge.idx = i
	sd := m.merge.sides[i]
	m.left, m.leftNL, m.leftPath, m.leftName = sd.lines, sd.nl, sd.path, sd.name
	m.savedL = m.left
	m.applied = nil
}
