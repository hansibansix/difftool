package tui

import (
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// pollMsg drives the once-a-second poll of the notes sidecar and of the
// compared files, so edits made outside difftool (an agent, an editor)
// show up without reopening.
type pollMsg struct{}

const pollEvery = time.Second

func pollTick() tea.Cmd {
	return tea.Tick(pollEvery, func(time.Time) tea.Msg { return pollMsg{} })
}

func modTime(path string) time.Time {
	st, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}

// stampDisk remembers the on-disk state the view currently shows.
func (m *model) stampDisk() {
	m.leftMod, m.rightMod = modTime(m.leftPath), modTime(m.rightPath)
	m.diskStale = false
}

// checkDisk reloads the view when a compared file changed on disk and
// reports whether it did. Unsaved in-memory changes are never discarded:
// the status says so once and the reload waits for save or undo. An open
// composer, selection or search is left alone too.
func (m *model) checkDisk() bool {
	if modTime(m.leftPath).Equal(m.leftMod) && modTime(m.rightPath).Equal(m.rightMod) {
		return false
	}
	if m.noteInput || m.visual || m.searchInput {
		return false
	}
	if m.dirty() {
		if !m.diskStale {
			m.diskStale = true
			m.status = "file changed on disk — " + keys.file.first("save") + " to save or " + keys.file.first("undo") + " to undo, then it reloads"
		}
		return false
	}
	m.reload(nil)
	m.status = "reloaded: file changed on disk"
	return true
}
