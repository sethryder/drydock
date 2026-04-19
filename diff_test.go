package main

import (
	"bytes"
	"sort"
	"strings"
	"testing"
)

func sortEntries(es []diffEntry) {
	sort.SliceStable(es, func(i, j int) bool { return es[i].path < es[j].path })
}

func TestDiffMaps_AddRemoveModify(t *testing.T) {
	a := map[string]interface{}{
		"keep":   1,
		"change": "old",
		"remove": true,
	}
	b := map[string]interface{}{
		"keep":   1,
		"change": "new",
		"add":    42,
	}
	got := diffMaps("", a, b)
	sortEntries(got)

	want := []diffEntry{
		{kind: changeAdd, path: "add", new: 42},
		{kind: changeModify, path: "change", old: "old", new: "new"},
		{kind: changeRemove, path: "remove", old: true},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].kind != want[i].kind || got[i].path != want[i].path {
			t.Errorf("entry %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDiffMaps_NestedRecursesWithDottedPaths(t *testing.T) {
	a := map[string]interface{}{
		"image": map[string]interface{}{"tag": "1", "repo": "x"},
	}
	b := map[string]interface{}{
		"image": map[string]interface{}{"tag": "2", "repo": "x"},
	}
	got := diffMaps("", a, b)
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1: %#v", len(got), got)
	}
	if got[0].path != "image.tag" || got[0].kind != changeModify {
		t.Fatalf("got %+v", got[0])
	}
}

func TestDiffSlices_EqualLengthDiffsElementwise(t *testing.T) {
	a := []interface{}{"a", "b", "c"}
	b := []interface{}{"a", "B", "c"}
	got := diffSlices("list", a, b)
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1: %#v", len(got), got)
	}
	if got[0].path != "list[1]" || got[0].kind != changeModify {
		t.Fatalf("got %+v", got[0])
	}
}

func TestDiffSlices_DifferentLengthReplacesWholeList(t *testing.T) {
	a := []interface{}{"a", "b"}
	b := []interface{}{"a", "b", "c"}
	got := diffSlices("list", a, b)
	if len(got) != 1 || got[0].path != "list" || got[0].kind != changeModify {
		t.Fatalf("got %#v", got)
	}
}

func TestDiffSlices_EqualReturnsNothing(t *testing.T) {
	got := diffSlices("list", []interface{}{1, 2}, []interface{}{1, 2})
	if len(got) != 0 {
		t.Fatalf("got %#v, want none", got)
	}
}

func TestRenderRelease_NoValueChanges(t *testing.T) {
	rc := HelmReleaseChange{
		Address: "helm_release.x",
		Actions: []string{"update"},
		Before:  map[string]interface{}{"a": 1},
		After:   map[string]interface{}{"a": 1},
	}
	var buf bytes.Buffer
	RenderRelease(&buf, rc, RenderOptions{Color: false})
	if !strings.Contains(buf.String(), "no value changes") {
		t.Fatalf("missing no-change marker:\n%s", buf.String())
	}
}

func TestRenderRelease_PlainOutput(t *testing.T) {
	rc := HelmReleaseChange{
		Address: "helm_release.airflow",
		Chart:   "airflow",
		Actions: []string{"update"},
		Before:  map[string]interface{}{"image": map[string]interface{}{"tag": "1"}, "old": true},
		After:   map[string]interface{}{"image": map[string]interface{}{"tag": "2"}, "new": 5},
	}
	var buf bytes.Buffer
	RenderRelease(&buf, rc, RenderOptions{Color: false})
	out := buf.String()

	for _, want := range []string{
		"helm_release.airflow",
		"chart=airflow",
		"+ new: 5",
		"- old: true",
		"~ image.tag:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("expected no ANSI codes, got:\n%s", out)
	}
}

func TestStripANSI(t *testing.T) {
	in := "\x1b[31mred\x1b[0m and \x1b[1mbold\x1b[0m"
	got := stripANSI(in)
	if got != "red and bold" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatValue(t *testing.T) {
	cases := []struct {
		in   interface{}
		want string
	}{
		{nil, "<nil>"},
		{"hi", `"hi"`},
		{42, "42"},
		{true, "true"},
	}
	for _, c := range cases {
		if got := formatValue(c.in); got != c.want {
			t.Errorf("formatValue(%#v) = %q, want %q", c.in, got, c.want)
		}
	}
	if !strings.HasPrefix(formatValue("a\nb"), "|\n") {
		t.Errorf("multiline string should use yaml-block style")
	}
}
