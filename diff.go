package main

import (
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
)

type changeKind int

const (
	changeAdd changeKind = iota
	changeRemove
	changeModify
)

type diffEntry struct {
	kind changeKind
	path string
	old  interface{}
	new  interface{}
}

type RenderOptions struct {
	Color bool
}

// RenderRelease writes a human-readable diff for a single helm_release change.
func RenderRelease(w io.Writer, rc HelmReleaseChange, opts RenderOptions) {
	title := fmt.Sprintf("%s  [%s]", rc.Address, strings.Join(rc.Actions, ","))
	if rc.Chart != "" {
		title += "  chart=" + rc.Chart
	}
	fmt.Fprintln(w, bold(opts, title))
	fmt.Fprintln(w, strings.Repeat("─", len(stripANSI(title))))

	entries := diffMaps("", rc.Before, rc.After)
	if len(entries) == 0 {
		fmt.Fprintln(w, "  (no value changes — only metadata differs)")
		fmt.Fprintln(w)
		return
	}

	sort.SliceStable(entries, func(i, j int) bool { return entries[i].path < entries[j].path })
	for _, e := range entries {
		writeEntry(w, e, opts)
	}
	fmt.Fprintln(w)
}

func diffMaps(prefix string, a, b map[string]interface{}) []diffEntry {
	var out []diffEntry
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}

	for k := range seen {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		av, aok := a[k]
		bv, bok := b[k]
		switch {
		case aok && !bok:
			out = append(out, diffEntry{kind: changeRemove, path: path, old: av})
		case !aok && bok:
			out = append(out, diffEntry{kind: changeAdd, path: path, new: bv})
		default:
			out = append(out, diffValues(path, av, bv)...)
		}
	}
	return out
}

func diffValues(path string, a, b interface{}) []diffEntry {
	am, aMap := a.(map[string]interface{})
	bm, bMap := b.(map[string]interface{})
	if aMap && bMap {
		return diffMaps(path, am, bm)
	}

	as, aSlice := a.([]interface{})
	bs, bSlice := b.([]interface{})
	if aSlice && bSlice {
		return diffSlices(path, as, bs)
	}

	if reflect.DeepEqual(a, b) {
		return nil
	}
	return []diffEntry{{kind: changeModify, path: path, old: a, new: b}}
}

func diffSlices(path string, a, b []interface{}) []diffEntry {
	if reflect.DeepEqual(a, b) {
		return nil
	}
	// Element-wise when lengths match; otherwise just show the whole list
	// replaced. Good enough for most Helm values lists.
	if len(a) == len(b) {
		var out []diffEntry
		for i := range a {
			p := fmt.Sprintf("%s[%d]", path, i)
			out = append(out, diffValues(p, a[i], b[i])...)
		}
		return out
	}
	return []diffEntry{{kind: changeModify, path: path, old: a, new: b}}
}

func writeEntry(w io.Writer, e diffEntry, opts RenderOptions) {
	switch e.kind {
	case changeAdd:
		fmt.Fprintf(w, "  %s %s: %s\n", green(opts, "+"), e.path, formatValue(e.new))
	case changeRemove:
		fmt.Fprintf(w, "  %s %s: %s\n", red(opts, "-"), e.path, formatValue(e.old))
	case changeModify:
		fmt.Fprintf(w, "  %s %s: %s %s %s\n",
			yellow(opts, "~"), e.path,
			formatValue(e.old), dim(opts, "→"), formatValue(e.new))
	}
}

func formatValue(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return "<nil>"
	case string:
		if strings.ContainsAny(t, "\n\r") {
			lines := strings.Split(t, "\n")
			return "|\n      " + strings.Join(lines, "\n      ")
		}
		return fmt.Sprintf("%q", t)
	case map[string]interface{}, []interface{}:
		return fmt.Sprintf("%v", t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// --- ANSI helpers ---------------------------------------------------------

func green(o RenderOptions, s string) string  { return color(o, "\x1b[32m", s) }
func red(o RenderOptions, s string) string    { return color(o, "\x1b[31m", s) }
func yellow(o RenderOptions, s string) string { return color(o, "\x1b[33m", s) }
func bold(o RenderOptions, s string) string   { return color(o, "\x1b[1m", s) }
func dim(o RenderOptions, s string) string    { return color(o, "\x1b[2m", s) }

func color(o RenderOptions, code, s string) string {
	if !o.Color {
		return s
	}
	return code + s + "\x1b[0m"
}

func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		if r == '\x1b' {
			inEsc = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
