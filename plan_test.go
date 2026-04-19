package main

import (
	"encoding/json"
	"os"
	"testing"
)

func loadSamplePlan(t *testing.T) *Plan {
	t.Helper()
	raw, err := os.ReadFile("testdata/sample.json")
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	var p Plan
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return &p
}

func TestHelmReleaseChanges_SkipsNoopAndNonHelm(t *testing.T) {
	p := loadSamplePlan(t)
	got := HelmReleaseChanges(p)
	if len(got) != 1 {
		t.Fatalf("got %d releases, want 1: %#v", len(got), got)
	}
	if got[0].Address != "helm_release.airflow" {
		t.Fatalf("got address %q", got[0].Address)
	}
	if got[0].Chart != "airflow" {
		t.Fatalf("got chart %q", got[0].Chart)
	}
}

func TestHelmReleaseChanges_MergesValuesAndSet(t *testing.T) {
	p := loadSamplePlan(t)
	rc := HelmReleaseChanges(p)[0]

	// values doc 1 sets executor; after should reflect KubernetesExecutor.
	if rc.After["executor"] != "KubernetesExecutor" {
		t.Errorf("after.executor = %v, want KubernetesExecutor", rc.After["executor"])
	}
	if rc.Before["executor"] != "CeleryExecutor" {
		t.Errorf("before.executor = %v, want CeleryExecutor", rc.Before["executor"])
	}

	// `set` block on `after` should add ingress.host on top of values.
	ing, ok := rc.After["ingress"].(map[string]interface{})
	if !ok {
		t.Fatalf("after.ingress missing or wrong type: %#v", rc.After["ingress"])
	}
	if ing["enabled"] != true {
		t.Errorf("ingress.enabled = %v, want true (coerced)", ing["enabled"])
	}
	if ing["host"] != "airflow.example.com" {
		t.Errorf("ingress.host = %v", ing["host"])
	}

	// scheduler.replicas merged from second values doc.
	sched := rc.After["scheduler"].(map[string]interface{})
	if sched["replicas"] != 2 {
		t.Errorf("scheduler.replicas = %v, want 2", sched["replicas"])
	}
}

func TestIsHelmRelease(t *testing.T) {
	cases := []struct {
		rc   ResourceChange
		want bool
	}{
		{ResourceChange{Mode: "managed", Type: "helm_release"}, true},
		{ResourceChange{Mode: "managed", Type: "helm_release_v2"}, true},
		{ResourceChange{Mode: "data", Type: "helm_release"}, false},
		{ResourceChange{Mode: "managed", Type: "aws_s3_bucket"}, false},
	}
	for _, c := range cases {
		if got := isHelmRelease(c.rc); got != c.want {
			t.Errorf("isHelmRelease(%+v) = %v, want %v", c.rc, got, c.want)
		}
	}
}

func TestIsNoop(t *testing.T) {
	if !isNoop([]string{"no-op"}) {
		t.Error("[no-op] should be noop")
	}
	if isNoop([]string{"update"}) {
		t.Error("[update] should not be noop")
	}
	if isNoop([]string{"no-op", "update"}) {
		t.Error("multi-action should not be noop")
	}
}

func TestChartName_PrefersAfterThenBefore(t *testing.T) {
	got := chartName(map[string]interface{}{"chart": "new"}, map[string]interface{}{"chart": "old"})
	if got != "new" {
		t.Errorf("got %q, want new", got)
	}
	got = chartName(nil, map[string]interface{}{"chart": "old"})
	if got != "old" {
		t.Errorf("got %q, want old", got)
	}
	got = chartName(nil, nil)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
