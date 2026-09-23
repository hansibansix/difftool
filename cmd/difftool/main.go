// Command difftool is a side-by-side terminal diff viewer with chunk-wise
// apply; see the README. This file only turns flags into tui.Options.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"difftool/internal/tui"
)

func main() {
	defTheme := tui.LoadConfig()
	if env := os.Getenv("DIFFTOOL_THEME"); env != "" {
		defTheme = env
	}
	themeName := flag.String("theme", defTheme, "color theme")
	gitMode := flag.Bool("git", false, "compare working tree against a git ref (default HEAD)")
	mergeMode := flag.Bool("merge", false, "3-way merge: -merge LOCAL BASE REMOTE MERGED (git mergetool)")
	exclude := flag.String("x", "", "additional ignore patterns, comma-separated globs")
	showKeys := flag.Bool("keys", false, "print the key bindings as config.json snippet and exit")
	notesPath := flag.String("notes", "", "agent notes sidecar (hunk --agent-context JSON); default: .difftool-notes.json in the repo root or cwd when present")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: difftool [-theme name] <left> <right>  (two files or two directories)")
		fmt.Fprintln(os.Stderr, "       difftool [-theme name] -git [ref] [path]  (working tree vs. git ref)")
		fmt.Fprintln(os.Stderr, "       difftool [-theme name] -git A..B [path]   (two git refs, read-only)")
		fmt.Fprintln(os.Stderr, "       difftool [-theme name] -git C^!  [path]   (one commit vs. its parent)")
		fmt.Fprintln(os.Stderr, "       difftool -merge LOCAL BASE REMOTE MERGED    (git mergetool)")
		fmt.Fprintln(os.Stderr, "       -notes file.json in any mode shows agent notes beside the code")
		fmt.Fprintf(os.Stderr, "themes: %s\n", tui.ThemeNames())
	}
	flag.Parse()
	if *showKeys {
		fmt.Println(tui.KeysJSON())
		return
	}
	opts := tui.Options{Theme: *themeName, Notes: *notesPath, Args: flag.Args()}
	switch {
	case *mergeMode:
		opts.Mode = tui.ModeMerge
	case *gitMode:
		opts.Mode = tui.ModeGit
	}
	for _, p := range strings.Split(*exclude, ",") {
		if p = strings.TrimSpace(p); p != "" {
			opts.Exclude = append(opts.Exclude, p)
		}
	}
	code, err := tui.Run(opts)
	switch {
	case errors.Is(err, tui.ErrUsage):
		flag.Usage()
	case err != nil:
		fmt.Fprintln(os.Stderr, "difftool:", err)
	}
	os.Exit(code)
}
