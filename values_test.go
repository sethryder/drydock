package main

import (
	"reflect"
	"testing"
)

func TestRenderValues_MergesYAMLDocsInOrder(t *testing.T) {
	state := map[string]interface{}{
		"values": []interface{}{
			"a: 1\nb:\n  c: 2\n",
			"b:\n  d: 3\n",
		},
	}
	got, err := renderValues(state)
	if err != nil {
		t.Fatalf("renderValues: %v", err)
	}
	want := map[string]interface{}{
		"a": 1,
		"b": map[string]interface{}{"c": 2, "d": 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestRenderValues_LaterDocOverridesEarlier(t *testing.T) {
	state := map[string]interface{}{
		"values": []interface{}{
			"image:\n  tag: old\n",
			"image:\n  tag: new\n",
		},
	}
	got, err := renderValues(state)
	if err != nil {
		t.Fatalf("renderValues: %v", err)
	}
	tag := got["image"].(map[string]interface{})["tag"]
	if tag != "new" {
		t.Fatalf("tag = %v, want new", tag)
	}
}

func TestRenderValues_SetOverlaysOnTopOfValues(t *testing.T) {
	state := map[string]interface{}{
		"values": []interface{}{"image:\n  tag: from-yaml\n"},
		"set": []interface{}{
			map[string]interface{}{"name": "image.tag", "value": "from-set"},
			map[string]interface{}{"name": "ingress.enabled", "value": "true"},
		},
	}
	got, err := renderValues(state)
	if err != nil {
		t.Fatalf("renderValues: %v", err)
	}
	if got["image"].(map[string]interface{})["tag"] != "from-set" {
		t.Fatalf("set did not override values: %#v", got["image"])
	}
	if got["ingress"].(map[string]interface{})["enabled"] != true {
		t.Fatalf("string \"true\" was not coerced to bool: %#v", got["ingress"])
	}
}

func TestRenderValues_SetSensitiveWinsOverSet(t *testing.T) {
	state := map[string]interface{}{
		"set": []interface{}{
			map[string]interface{}{"name": "password", "value": "plain"},
		},
		"set_sensitive": []interface{}{
			map[string]interface{}{"name": "password", "value": "secret"},
		},
	}
	got, err := renderValues(state)
	if err != nil {
		t.Fatalf("renderValues: %v", err)
	}
	if got["password"] != "secret" {
		t.Fatalf("password = %v, want secret", got["password"])
	}
}

func TestRenderValues_SetListWritesSlice(t *testing.T) {
	state := map[string]interface{}{
		"set_list": []interface{}{
			map[string]interface{}{
				"name":  "tolerations",
				"value": []interface{}{"a", "b", "c"},
			},
		},
	}
	got, err := renderValues(state)
	if err != nil {
		t.Fatalf("renderValues: %v", err)
	}
	want := []interface{}{"a", "b", "c"}
	if !reflect.DeepEqual(got["tolerations"], want) {
		t.Fatalf("tolerations = %#v, want %#v", got["tolerations"], want)
	}
}

func TestRenderValues_NilStateYieldsEmptyMap(t *testing.T) {
	got, err := renderValues(nil)
	if err != nil {
		t.Fatalf("renderValues: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %#v, want empty", got)
	}
}

func TestRenderValues_SkipsBlankValuesDocs(t *testing.T) {
	state := map[string]interface{}{
		"values": []interface{}{"", "   \n", "a: 1\n"},
	}
	got, err := renderValues(state)
	if err != nil {
		t.Fatalf("renderValues: %v", err)
	}
	if got["a"] != 1 {
		t.Fatalf("got %#v, want a=1", got)
	}
}

func TestRenderValues_BadYAMLReturnsError(t *testing.T) {
	state := map[string]interface{}{
		"values": []interface{}{"a: [unterminated\n"},
	}
	if _, err := renderValues(state); err == nil {
		t.Fatal("expected error from malformed YAML, got nil")
	}
}

func TestCoerceScalar(t *testing.T) {
	cases := []struct {
		in   interface{}
		want interface{}
	}{
		{"true", true},
		{"false", false},
		{"42", int64(42)},
		{"3.14", 3.14},
		{"hello", "hello"},
		{"", ""},
		{42, 42}, // non-string passes through unchanged
		{nil, nil},
	}
	for _, c := range cases {
		got := coerceScalar(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("coerceScalar(%#v) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func TestDeepMerge(t *testing.T) {
	dst := map[string]interface{}{
		"a": 1,
		"b": map[string]interface{}{"x": 1, "y": 2},
		"c": []interface{}{1, 2},
	}
	src := map[string]interface{}{
		"a": 99,                                       // scalar replace
		"b": map[string]interface{}{"y": 22, "z": 3}, // nested merge
		"c": []interface{}{9},                        // slice replaced wholesale
		"d": "new",                                   // new key
	}
	got := deepMerge(dst, src)
	want := map[string]interface{}{
		"a": 99,
		"b": map[string]interface{}{"x": 1, "y": 22, "z": 3},
		"c": []interface{}{9},
		"d": "new",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestDeepMerge_NilDst(t *testing.T) {
	got := deepMerge(nil, map[string]interface{}{"a": 1})
	if got["a"] != 1 {
		t.Fatalf("got %#v", got)
	}
}

func TestSetAtPath_CreatesNestedMaps(t *testing.T) {
	m := map[string]interface{}{}
	setAtPath(m, "a.b.c", 42)
	got := m["a"].(map[string]interface{})["b"].(map[string]interface{})["c"]
	if got != 42 {
		t.Fatalf("got %v, want 42", got)
	}
}

func TestSetAtPath_OverwritesNonMapIntermediate(t *testing.T) {
	m := map[string]interface{}{"a": "scalar"}
	setAtPath(m, "a.b", 1)
	if m["a"].(map[string]interface{})["b"] != 1 {
		t.Fatalf("got %#v", m)
	}
}
