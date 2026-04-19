package main

import "strings"

// Plan is a minimal subset of the `terraform show -json` schema — just what we
// need to extract helm_release values. The full schema lives at
// https://developer.hashicorp.com/terraform/internals/json-format.
type Plan struct {
	FormatVersion   string           `json:"format_version"`
	ResourceChanges []ResourceChange `json:"resource_changes"`
}

type ResourceChange struct {
	Address      string `json:"address"`
	ModuleAddr   string `json:"module_address,omitempty"`
	Mode         string `json:"mode"`
	Type         string `json:"type"`
	Name         string `json:"name"`
	ProviderName string `json:"provider_name"`
	Change       Change `json:"change"`
}

type Change struct {
	Actions []string               `json:"actions"`
	Before  map[string]interface{} `json:"before"`
	After   map[string]interface{} `json:"after"`
}

// HelmReleaseChange is the distilled view of a single helm_release change.
type HelmReleaseChange struct {
	Address string
	Chart   string
	Actions []string
	// Merged `values` map (YAML-decoded and deep-merged in list order),
	// plus the flat `set`/`set_sensitive`/`set_list` overrides applied on top.
	Before map[string]interface{}
	After  map[string]interface{}
}

// HelmReleaseChanges extracts all helm_release resource changes from the plan.
func HelmReleaseChanges(p *Plan) []HelmReleaseChange {
	var out []HelmReleaseChange
	for _, rc := range p.ResourceChanges {
		if !isHelmRelease(rc) {
			continue
		}
		if isNoop(rc.Change.Actions) {
			continue
		}
		before, err := renderValues(rc.Change.Before)
		if err != nil {
			before = map[string]interface{}{"__parse_error__": err.Error()}
		}
		after, err := renderValues(rc.Change.After)
		if err != nil {
			after = map[string]interface{}{"__parse_error__": err.Error()}
		}
		out = append(out, HelmReleaseChange{
			Address: rc.Address,
			Chart:   chartName(rc.Change.After, rc.Change.Before),
			Actions: rc.Change.Actions,
			Before:  before,
			After:   after,
		})
	}
	return out
}

func isHelmRelease(rc ResourceChange) bool {
	if rc.Mode != "managed" {
		return false
	}
	// Covers `helm_release` from hashicorp/helm and any future variants.
	return rc.Type == "helm_release" || strings.HasPrefix(rc.Type, "helm_release_")
}

func isNoop(actions []string) bool {
	return len(actions) == 1 && actions[0] == "no-op"
}

func chartName(after, before map[string]interface{}) string {
	for _, m := range []map[string]interface{}{after, before} {
		if m == nil {
			continue
		}
		if v, ok := m["chart"].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
