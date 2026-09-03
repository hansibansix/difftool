// Package notes reads and writes the agent notes sidecar: a JSON file in
// hunk's --agent-context format (https://github.com/modem-dev/hunk)
// anchoring remarks to lines of the compared files, with difftool's
// extensions (ranges, resolved, text anchors).
package notes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Note struct {
	FilePath string `json:"filePath"`
	NewLine  int    `json:"newLine,omitempty"` // 1-based line of the right side
	OldLine  int    `json:"oldLine,omitempty"` // 1-based line of the left side
	Summary  string `json:"summary"`
	Author   string `json:"author,omitempty"`
	// difftool extensions to hunk's format: a range ends at endLine on the
	// anchored side; resolved notes collapse to one line; match is the text
	// of the anchored line, which wins over the line numbers when it is
	// found exactly once, so notes survive edits and agents need no numbers
	EndLine  int    `json:"endLine,omitempty"`
	Resolved bool   `json:"resolved,omitempty"`
	Match    string `json:"match,omitempty"`
}

// line returns the anchor line and whether it is on the right side.
func (n *Note) Line() (int, bool) {
	if n.NewLine > 0 {
		return n.NewLine, true
	}
	return n.OldLine, false
}

// covers reports whether the Note's line range includes 1-based line ln on
// the given side.
func (n *Note) Covers(ln int, right bool) bool {
	start, r := n.Line()
	return start > 0 && r == right && ln >= start && ln <= max(start, n.EndLine)
}

// File is the sidecar's JSON shape.
type File struct {
	Comments []Note `json:"comments"`
}

// Store is the loaded sidecar; an empty Path means the feature is off.
type Store struct {
	Path  string
	Items []Note
	mtime time.Time
}

// Load reads the sidecar; a missing file is an empty set (it is
// created by the first Note typed in difftool).
func (s *Store) Load(path string) error {
	s.Path, s.Items, s.mtime = path, nil, time.Time{}
	st, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	s.Items, s.mtime = f.Comments, st.ModTime()
	return nil
}

// Reload re-reads the sidecar when its mtime changed and reports
// whether it did.
func (s *Store) Reload() bool {
	st, err := os.Stat(s.Path)
	if err != nil || st.ModTime().Equal(s.mtime) {
		return false
	}
	return s.Load(s.Path) == nil
}

func (s *Store) Save() error {
	data, err := json.MarshalIndent(File{s.Items}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(s.Path, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if st, err := os.Stat(s.Path); err == nil {
		s.mtime = st.ModTime() // our own write is not a reload
	}
	return nil
}

// Mtime is the modification time of the file as last read or written.
func (s *Store) Mtime() time.Time { return s.mtime }

// Matches reports whether a Note's filePath names key. Agents write
// paths relative to the repository or project root while the key is what
// difftool knows (repo-relative in git mode, root-relative in dir mode,
// cwd-relative for two files), so either may carry extra leading dirs.
func Matches(filePath, key string) bool {
	return filePath == key || strings.HasSuffix(key, "/"+filePath) || strings.HasSuffix(filePath, "/"+key)
}

// For returns pointers to the notes of one file, in file order.
func (s *Store) For(key string) []*Note {
	var out []*Note
	for i := range s.Items {
		if Matches(s.Items[i].FilePath, key) {
			out = append(out, &s.Items[i])
		}
	}
	return out
}

// Count counts the open (unresolved) notes of a file.
func (s *Store) Count(key string) int {
	n := 0
	for _, x := range s.For(key) {
		if !x.Resolved {
			n++
		}
	}
	return n
}

// FindLine returns the 0-based index of the one line whose trimmed text
// equals text, else the one line containing it, else -1.
func FindLine(lines []string, text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return -1
	}
	for _, eq := range []bool{true, false} {
		hit := -1
		for i, l := range lines {
			if (eq && strings.TrimSpace(l) == text) || (!eq && strings.Contains(l, text)) {
				if hit >= 0 {
					hit = -2 // ambiguous
					break
				}
				hit = i
			}
		}
		if hit >= 0 {
			return hit
		}
	}
	return -1
}
