package tui

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain redirects the config file so tests that toggle settings (and
// thereby save) never overwrite the developer's real config.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "difftool-test-")
	if err != nil {
		panic(err)
	}
	cfgPath = filepath.Join(dir, "config.json")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
