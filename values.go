package main

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// renderValues turns the `before`/`after` map of a helm_release resource into
// the effective merged values map that Helm would actually see.
//
// It merges in the same order Helm does:
//  1. each string in `values` (later wins)
//  2. `set` blocks (flat key=value, dotted paths)
//  3. `set_list` blocks
//  4. `set_sensitive` blocks (highest precedence)
//
// A nil input (e.g. a resource being created or destroyed) yields an empty map.
func renderValues(state map[string]interface{}) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	if state == nil {
		return out, nil
	}

	if raw, ok := state["values"]; ok {
		list, err := asStringList(raw)
		if err != nil {
			return nil, fmt.Errorf("values: %w", err)
		}
		for i, doc := range list {
			if strings.TrimSpace(doc) == "" {
				continue
			}
			var m map[string]interface{}
			if err := yaml.Unmarshal([]byte(doc), &m); err != nil {
				return nil, fmt.Errorf("values[%d]: %w", i, err)
			}
			out = deepMerge(out, m)
		}
	}

	for _, key := range []string{"set", "set_list", "set_sensitive"} {
		raw, ok := state[key]
		if !ok || raw == nil {
			continue
		}
		blocks, ok := raw.([]interface{})
		if !ok {
			continue
		}
		for _, b := range blocks {
			bm, ok := b.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := bm["name"].(string)
			if name == "" {
				continue
			}
			val := bm["value"]
			if key == "set_list" {
				if lst, ok := val.([]interface{}); ok {
					setAtPath(out, name, lst)
					continue
				}
			}
			setAtPath(out, name, coerceScalar(val))
		}
	}

	return out, nil
}

func asStringList(raw interface{}) ([]string, error) {
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case []interface{}:
		out := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("item %d is %T, want string", i, item)
			}
			out = append(out, s)
		}
		return out, nil
	case []string:
		return v, nil
	default:
		return nil, fmt.Errorf("unexpected %T", raw)
	}
}

// deepMerge merges src into dst, returning the result. Maps are merged
// recursively; every other type in src replaces dst wholesale (matches Helm).
func deepMerge(dst, src map[string]interface{}) map[string]interface{} {
	if dst == nil {
		dst = map[string]interface{}{}
	}
	for k, sv := range src {
		if dv, ok := dst[k]; ok {
			if dm, dok := dv.(map[string]interface{}); dok {
				if sm, sok := sv.(map[string]interface{}); sok {
					dst[k] = deepMerge(dm, sm)
					continue
				}
			}
		}
		dst[k] = sv
	}
	return dst
}

// setAtPath writes v into m at a Helm-style dotted path. It does not try to
// parse array indices (Helm's `set` uses `a[0].b` but the provider normalizes
// most of this; we keep it simple).
func setAtPath(m map[string]interface{}, path string, v interface{}) {
	parts := strings.Split(path, ".")
	cur := m
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = v
			return
		}
		next, ok := cur[p].(map[string]interface{})
		if !ok {
			next = map[string]interface{}{}
			cur[p] = next
		}
		cur = next
	}
}

// coerceScalar tries to interpret Terraform-stringified scalars (numbers,
// bools) back to their native types so the diff doesn't show spurious
// "123" -> 123 noise.
func coerceScalar(v interface{}) interface{} {
	s, ok := v.(string)
	if !ok {
		return v
	}
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}
