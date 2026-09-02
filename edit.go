package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// editDoneMsg arrives when the external editor has exited.
type editDoneMsg struct{ err error }

// editIn opens one side in $VISUAL / $EDITOR at the current hunk. In-memory
// changes must be saved (or undone) first so the editor sees the same file.
func (m *model) editIn(right bool) tea.Cmd {
	if (right && m.roRight) || (!right && m.roLeft) {
		m.status = "that side is read-only (git ref)"
		return nil
	}
	if m.dirty() {
		m.status = "save (s) or undo (u) before editing"
		return nil
	}
	path, line := m.leftPath, 0
	if right {
		path = m.rightPath
	}
	if len(m.nav) > 0 {
		if t := m.nav[m.cur]; t.ci >= 0 {
			c := m.chunks[t.ci]
			line = c.l0
			if right {
				line = c.r0
			}
		} else {
			a := m.applied[t.ai]
			line = a.l0
			if right {
				line = a.r0
			}
		}
	}
	return tea.ExecProcess(editorCmd(path, line+1), func(err error) tea.Msg { return editDoneMsg{err} })
}

// editorCmd builds the editor invocation; editors known to take +N get the
// cursor line. $VISUAL and $EDITOR may carry arguments ("code --wait").
func editorCmd(path string, line int) *exec.Cmd {
	args := strings.Fields(envOr("VISUAL", envOr("EDITOR", "vi")))
	switch filepath.Base(args[0]) {
	case "vi", "vim", "nvim", "nano", "micro", "emacs", "kak":
		args = append(args, fmt.Sprintf("+%d", line))
	}
	args = append(args, path)
	return exec.Command(args[0], args[1:]...)
}

// reload re-reads both files after an external edit. Applied markers and
// undo history refer to the old content and are dropped.
func (m *model) reload(editErr error) {
	left, leftNL, err := readLines(m.leftPath)
	if err == nil {
		m.left, m.leftNL, m.savedL = left, leftNL, left
		var right []string
		var rightNL bool
		if right, rightNL, err = readLines(m.rightPath); err == nil {
			m.right, m.rightNL, m.savedR = right, rightNL, right
		}
	}
	m.applied, m.undo = nil, nil
	m.recompute()
	switch {
	case editErr != nil:
		m.status = "editor: " + editErr.Error()
	case err != nil:
		m.status = "error: " + err.Error()
	default:
		m.status = "reloaded after edit"
	}
}
