package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Key bindings. Each context (file view, directory view, everywhere) maps
// action names to key names as bubbletea reports them ("ctrl+d", "enter",
// "esc", "tab", "up", " " for space). Users override single actions via the
// "keys" object in config.json; the order of the defaults decides which
// action wins when a key is bound twice.

type binding struct {
	action string
	keys   []string
}

type keymap []binding

// action returns the action bound to key, or "".
func (k keymap) action(key string) string {
	for _, b := range k {
		for _, x := range b.keys {
			if x == key {
				return b.action
			}
		}
	}
	return ""
}

// first returns the primary key of an action, for hints.
func (k keymap) first(action string) string {
	for _, b := range k {
		if b.action == action && len(b.keys) > 0 {
			return b.keys[0]
		}
	}
	return ""
}

// all returns every key of an action, for the help overlay.
func (k keymap) all(action string) []string {
	for _, b := range k {
		if b.action == action {
			return b.keys
		}
	}
	return nil
}

func (k keymap) set(action string, ks []string) bool {
	for i := range k {
		if k[i].action == action {
			k[i].keys = ks
			return true
		}
	}
	return false
}

type keymaps struct{ file, dir, global keymap }

var keys = defaultKeys()

func defaultKeys() keymaps {
	return keymaps{
		file: keymap{
			{"next", []string{"n", "]"}},
			{"prev", []string{"p", "["}},
			{"apply-right", []string{"l", "right", ">"}},
			{"apply-left", []string{"h", "left", "<"}},
			{"apply-all", []string{"a"}},
			{"visual", []string{"v"}},
			{"reset", []string{"x"}},
			{"reset-all", []string{"X"}},
			{"undo", []string{"u"}},
			{"save", []string{"s"}},
			{"edit-right", []string{"e"}},
			{"edit-left", []string{"E"}},
			{"patch", []string{"P"}},
			{"search", []string{"/"}},
			{"search-prev", []string{"N"}},
			{"intraline", []string{"i"}},
			{"wrap", []string{"w"}},
			{"unified", []string{"o"}},
			{"fold", []string{"z"}},
			{"next-file", []string{"J"}},
			{"prev-file", []string{"K"}},
			{"merge-local", []string{"1"}},
			{"merge-base", []string{"2"}},
			{"merge-remote", []string{"3"}},
			{"down", []string{"j", "down"}},
			{"up", []string{"k", "up"}},
			{"half-down", []string{"ctrl+d"}},
			{"half-up", []string{"ctrl+u"}},
			{"top", []string{"g"}},
			{"bottom", []string{"G"}},
			{"scroll-left", []string{"H"}},
			{"scroll-right", []string{"L"}},
			{"tree", []string{"tab"}},
			{"quit", []string{"q", "esc"}},
		},
		dir: keymap{
			{"open", []string{"enter", "tab"}},
			{"copy-right", []string{"l", "right", ">"}},
			{"copy-left", []string{"h", "left", "<"}},
			{"sync-all", []string{"A"}},
			{"undo", []string{"u"}},
			{"filter", []string{"/"}},
			{"identical", []string{"a"}},
			{"down", []string{"j", "down"}},
			{"up", []string{"k", "up"}},
			{"half-down", []string{"ctrl+d"}},
			{"half-up", []string{"ctrl+u"}},
			{"top", []string{"g"}},
			{"bottom", []string{"G"}},
			{"quit", []string{"q", "esc"}},
		},
		global: keymap{
			{"settings", []string{","}},
			{"help", []string{"?"}},
			{"tree-toggle", []string{"t"}},
			{"ignore-add", []string{"I"}},
		},
	}
}

// applyKeys overrides the defaults with the user's "keys" config and
// returns a note per unknown context or action.
func applyKeys(user map[string]map[string][]string) []string {
	keys = defaultKeys()
	var warn []string
	ctxs := map[string]keymap{"file": keys.file, "dir": keys.dir, "global": keys.global}
	for ctx, m := range user {
		km, ok := ctxs[ctx]
		if !ok {
			warn = append(warn, fmt.Sprintf("unknown key context %q (file, dir, global)", ctx))
			continue
		}
		for act, ks := range m {
			if !km.set(act, ks) {
				warn = append(warn, fmt.Sprintf("unknown key action %q in %q", act, ctx))
			}
		}
	}
	sort.Strings(warn)
	return warn
}

// keysJSON renders the current bindings as the config.json "keys" object.
func keysJSON() string {
	out := map[string]map[string][]string{}
	for ctx, km := range map[string]keymap{"file": keys.file, "dir": keys.dir, "global": keys.global} {
		out[ctx] = map[string][]string{}
		for _, b := range km {
			out[ctx][b.action] = b.keys
		}
	}
	data, _ := json.MarshalIndent(map[string]any{"keys": out}, "", "  ")
	return string(data)
}

// keyLabel formats the keys of an action for the help overlay ("n ]").
func keyLabel(km keymap, actions ...string) string {
	var parts []string
	for _, a := range actions {
		parts = append(parts, strings.Join(km.all(a), " "))
	}
	return strings.Join(parts, " / ")
}

// hint builds a footer hint from the primary keys of actions ("n/p").
func hint(km keymap, actions ...string) string {
	var ks []string
	for _, a := range actions {
		ks = append(ks, km.first(a))
	}
	return strings.Join(ks, "/")
}
