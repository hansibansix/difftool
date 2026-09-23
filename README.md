# difftool

Side-by-side terminal diff viewer with chunk-wise apply, in the spirit of
PhpStorm's diff window. Compares two files or two directories.

```
difftool [-theme name] <left> <right>       # two files or two directories
difftool [-theme name] -git [ref] [path]    # working tree vs. git ref (default HEAD)
difftool [-theme name] -git A..B [path]     # two refs, both read-only
difftool [-theme name] -git C^! [path]      # one commit against its parent
difftool -merge LOCAL BASE REMOTE MERGED    # 3-way merge (git mergetool)
difftool -notes notes.json ...              # any mode: show agent notes beside the code
difftool-review [difftool args]             # agents: open the review in a herdr tab, return when it closes
```

In git mode the left side is the ref version (read-only); applying a chunk
or copying a file left → right reverts it in the working tree. Untracked
files are listed as "only right". An optional path (file or directory)
limits the comparison; a single existing path is taken as the path, not a
ref — use the two-arg form to disambiguate. `A..B` compares two refs
without touching the working tree; `C^!` shows what commit C changed.

## Merge mode

3-way merge for `git mergetool`: the right pane is the merge result (seeded
by `git merge-file`; conflicts appear as marker blocks, each diffing as one
hunk), the left pane shows LOCAL, BASE or REMOTE (`1`/`2`/`3`). Applying a
hunk onto a conflict block resolves it with that side; `s` writes MERGED.
The exit code is 1 while conflicts remain, so enable `trustExitCode`:

```
[merge]
    tool = difftool
[mergetool "difftool"]
    cmd = difftool -merge "$LOCAL" "$BASE" "$REMOTE" "$MERGED"
    trustExitCode = true
```

## File view

| Key           | Action                              |
|---------------|-------------------------------------|
| `n` / `p`     | next / previous change              |
| `l` `→` `>`   | apply current chunk left → right    |
| `h` `←` `<`   | apply current chunk right → left    |
| `a`           | apply ALL pending (then `l`/`h`)    |
| `v`           | select lines (`j`/`k`), `l`/`h` applies them |
| `x` / `X`     | reset applied hunk / all            |
| `u`           | undo last apply / reset / all       |
| `/` `n`/`N`   | search, next/prev match             |
| `}` / `{`     | next / prev note (`-notes`)         |
| `c` / `C`     | note on the cursor line, or on the selected lines in visual mode (on a note: edit yours, reply to the agent's) / delete the note under the cursor |
| `r`           | resolve / reopen the note under the cursor |
| `A`           | hide / show all note boxes          |
| `J` / `K`     | next / prev file (dir mode)         |
| `s`           | save modified file(s)               |
| `e` / `E`     | edit the right / left file in `$VISUAL`/`$EDITOR` at the current hunk; the diff reloads on exit |
| `P`           | export pending hunks as a unified patch (clipboard via wl-copy/xclip/xsel/pbcopy, else `./difftool.patch`); in visual mode only the current hunk |
| `j` / `k`     | move the line cursor `▶` (the view follows), `ctrl+d`/`ctrl+u` half page |
| `H` / `L`     | horizontal scroll (long lines)      |
| `i`           | toggle intraline highlight          |
| `w`           | toggle line wrap (persisted)        |
| `o`           | toggle unified one-column view (persisted) |
| `z`           | fold unchanged lines to 3 of context; click a fold to expand it (persisted) |
| `,`           | settings menu                       |
| `g` / `G`     | top / bottom                        |
| `q` / `esc`   | quit (asks once on unsaved changes) |

## Directory view

Split layout: the file tree on the left, the diff of the selected file on
the right (the diff follows the selection; below 90 columns the panes
alternate full-screen instead). `.git` is skipped.

| Key         | Action                                   |
|-------------|------------------------------------------|
| `enter`/`tab` | focus the diff pane (`tab`/`q` returns) |
| `t`         | show / hide the tree pane (persisted)    |
| `/`         | filter the list (esc clears)             |
| `l` / `h`   | copy selected file to the other side; for a one-sided file the other direction deletes it (asks `y`) |
| `A`         | sync ALL listed files (`l`/`h`, then `y`)  |
| `I`         | add an ignore pattern (prefilled with the file name) |
| `u`         | undo the last copy / delete / sync       |
| `a`         | toggle showing identical files           |
| `j` / `k`   | move selection                           |
| `q` / `esc` | quit                                     |

Changed words within modified lines are emphasized (intraline diff).
A scrollbar strip on the right edge marks where changes and applied hunks
live in the file; line numbers are tinted by change type. Code is
syntax-highlighted (chroma, style matched to the theme; toggleable).
Mouse wheel scrolls (horizontal wheel too). `*` in the header marks unsaved changes.
Applied chunks stay tinted with a `▶`/`◀` arrow showing the copy direction,
remain reachable with `n`/`p`, and the view does not jump on apply.
Files made equal during the session stay listed as `✓ applied` in the
directory view.
The directory list groups files by folder with file icons, a status label
and a per-file diffstat (`+12 −3`); below 50 columns the label gives way
and the file name takes the status color. Mouse: click a file or hunk to select it,
click a pane to focus it, the wheel scrolls the pane under the pointer.

## Settings

`?` shows a help overlay with all keys.
`,` opens the settings menu in any view: theme, syntax + intraline highlight,
line wrap, unified view, folding, ignore whitespace, ignore blank-line
changes, ignore lines matching a regex, tab width, show identical files,
file icons in the tree (Nerd Font; turn off without one), ignore patterns on/off,
honour `.gitignore` in directory mode (git mode always does; the directory must
be inside a repository for its rules, including the global excludes file, to apply).
Changes apply live and are saved to `~/.config/difftool/config.json` on close.

Ignore rules: with *ignore blank-line changes* a hunk made only of blank
lines is shown plain and skipped by `n`/`p`, apply-all and patch export;
*ignore lines matching* (`ignore_regex`, e.g. `^\s*//` or `\$Id:`) does the
same for hunks whose lines all match. The footer counts them as `N ignored`.
In git mode the patch uses repo-relative `a/` `b/` paths, so
`P` + `git apply --cached` stages exactly the shown hunks.

## Agent notes

`-notes file.json` loads remarks in [hunk](https://github.com/modem-dev/hunk)'s
`--agent-context` format (without the flag, a `.difftool-notes.json` in the
repository root or the working directory is picked up) and shows each one as
a framed box above the line
it refers to, titled with its author and line (`}`/`{` jump between them,
the tree shows `✎N` per file, the scrollbar marks them). An agent that just edited a repo writes the file
before you review:

```json
{
  "comments": [
    { "filePath": "local/foo/lib.php", "newLine": 42, "summary": "Switched to delete_records_select: the old loop ran one query per row." },
    { "filePath": "local/foo/db/upgrade.php", "oldLine": 15, "summary": "Not sure the savepoint matches version.php, please check." },
    { "filePath": "local/foo/version.php", "summary": "File-level remark (no line)." }
  ]
}
```

`newLine` counts lines of the right side (working tree in git mode),
`oldLine` of the left; a note with neither sits at the top of the file.
Three difftool additions: `match` is the text of the anchored line and, when
it occurs exactly once in the file, wins over the line number (so agents
need not count lines and notes follow later edits); `endLine` turns the
anchor into a range on the same side (the covered line numbers take the
note's color, the title reads `L42-47`); `"resolved": true` collapses the
note to one dim line. Notes you write record `match` automatically.
`filePath` is matched by suffix against the repo-relative path (git mode),
the tree path (directory mode) or the cwd-relative path (two files), so
`lib.php` and `repo/local/foo/lib.php` both find `local/foo/lib.php`.
hunk's `hunk`/`hunkNumber` anchors and `markup` are not supported; such
notes show at the top of their file and lose those fields when the file is
rewritten.

The file is polled once a second: an agent can append notes while the diff
is open and they appear in place. Move the `▶` line cursor with `j`/`k` (or
click a line) and press `c`: a composer box opens above that line, `enter`
adds a line, `ctrl+s` saves, `esc` cancels. The note is written into the same
file with `"author": "human"` so the agent can read your feedback
afterwards. On a note row `c` edits your own note or replies to an agent's
(a new note on the same line), `C` deletes it, `r` resolves or reopens it
(so you can tick agent notes off while reading; the agent drops resolved
ones on its next pass). Select lines with `v` first and `c` writes a range
note. `A` hides all boxes while the gutter tint and tree badges stay.
Anchors follow applied and reset hunks within the session; they are not
rewritten when you edit the files externally.

The compared files are polled as well: when one changes on disk (an agent
or editor wrote it) the view reloads, dropping applied markers and undo
history like after `e`. Unsaved in-memory changes are never discarded; the
status asks you to save or undo first. In directory mode only the open
file is watched, the tree refreshes on the next selection.

On quit difftool prints `difftool: review closed · N notes from you · M
resolved` on stderr. `difftool-review` (a bash script, needs a running
[herdr](https://herdr.dev) session) builds the agent loop: it opens
difftool in a new herdr tab, blocks until the difftool process is gone,
closes the tab and prints a summary of the sidecar. An agent runs it in the background after writing its
notes, is woken when you quit, and reads your notes right away.

## Ignore patterns

Directory scans (and git mode) skip files and folders matching the glob
patterns in `ignore_patterns` (defaults: `node_modules`, `vendor`, VCS/IDE
metadata, `__pycache__`, minified assets, swap/OS junk); matching folders
are pruned entirely. Patterns without `/` match any path component (a
directory name hides its whole subtree), patterns with `/` match the
relative path. `-x 'pat,pat'` adds patterns for
one run; the settings menu toggles all patterns on/off, and `enter` on that
entry opens an editor to add (`a`) and delete (`d`) patterns.

## Key bindings

Every key in the tables above can be rebound in `config.json` under `keys`,
per context (`file`, `dir`, `global`) and action. Only the actions you list
change; the rest keep their defaults. `difftool -keys` prints the complete
current map as a snippet to start from, and the help overlay and footer
hints always show the active bindings.

```json
"keys": {
  "file": { "apply-right": ["l", "right"], "apply-left": ["h", "left"], "save": ["ctrl+s"] },
  "dir":  { "quit": ["q"] },
  "global": { "help": ["f1", "?"] }
}
```

Key names are bubbletea's: letters as typed, `ctrl+x`, `enter`, `esc`,
`tab`, `up`/`down`/`left`/`right`, `f1`…`f12`, `" "` for space (all lowercase). `ctrl+c` always quits
and `esc` always cancels an input. Unknown actions are reported on stderr at
startup.

## Themes

`rose-pine` (default), `catppuccin` (mocha), `nord`, `dracula`, `darcula`
(JetBrains), `gruvbox`, `tokyonight` — all truecolor — and `ansi` as
256-color fallback. Select in the settings menu, via `-theme <name>`, or
`DIFFTOOL_THEME=<name>` (precedence: flag > env > config).

## Build

```
go build -o ~/.local/bin/difftool ./cmd/difftool
cp scripts/difftool-review ~/.local/bin/       # optional: the herdr review wrapper
```

Layout: `cmd/difftool` parses flags, `internal/tui` is the application,
`internal/diff` the line and intraline diff, `internal/notes` the sidecar
format, `scripts/` the helper for agents.
