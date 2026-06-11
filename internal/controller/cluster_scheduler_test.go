/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import "testing"

func TestOrdinalFromPodName(t *testing.T) {
	cases := []struct {
		pod, cluster string
		want         int32
		ok           bool
	}{
		{"etl-small-0", "etl-small", 0, true},
		{"etl-small-12", "etl-small", 12, true},
		{"etl-small-x", "etl-small", 0, false},
		{"other-1", "etl-small", 0, false},
		{"etl-small", "etl-small", 0, false},
		{"etl-small--1", "etl-small", 0, false},
	}
	for _, tc := range cases {
		got, ok := ordinalFromPodName(tc.pod, tc.cluster)
		if ok != tc.ok || got != tc.want {
			t.Errorf("ordinalFromPodName(%q,%q) = (%d,%v), want (%d,%v)", tc.pod, tc.cluster, got, ok, tc.want, tc.ok)
		}
	}
}

func TestLeastLoadedInstance(t *testing.T) {
	// no ready instances
	if _, ok := leastLoadedInstance(nil, map[int32]int{}); ok {
		t.Errorf("expected ok=false with no ready instances")
	}
	// empty load → lowest ordinal
	got, ok := leastLoadedInstance([]int32{2, 0, 1}, map[int32]int{})
	if !ok || got != 0 {
		t.Errorf("expected ordinal 0 on empty load, got (%d,%v)", got, ok)
	}
	// uneven load → least loaded
	got, ok = leastLoadedInstance([]int32{0, 1, 2}, map[int32]int{0: 3, 1: 1, 2: 5})
	if !ok || got != 1 {
		t.Errorf("expected ordinal 1 (least loaded), got (%d,%v)", got, ok)
	}
	// tie → lowest ordinal
	got, ok = leastLoadedInstance([]int32{0, 1, 2}, map[int32]int{0: 2, 1: 2, 2: 2})
	if !ok || got != 0 {
		t.Errorf("expected ordinal 0 on tie, got (%d,%v)", got, ok)
	}
}

func TestPickInstance(t *testing.T) {
	// sticky: current instance still ready → reused, even if another is less loaded
	got, ok := pickInstance("c-1", "c", []int32{0, 1}, map[int32]int{0: 0, 1: 5})
	if !ok || got != 1 {
		t.Errorf("expected sticky reuse of ordinal 1, got (%d,%v)", got, ok)
	}
	// current instance no longer ready → fall through to least-loaded
	got, ok = pickInstance("c-2", "c", []int32{0, 1}, map[int32]int{0: 3, 1: 1})
	if !ok || got != 1 {
		t.Errorf("expected least-loaded ordinal 1, got (%d,%v)", got, ok)
	}
	// unplaced (empty current) → least-loaded (lowest on empty)
	got, ok = pickInstance("", "c", []int32{0, 1}, map[int32]int{})
	if !ok || got != 0 {
		t.Errorf("expected ordinal 0 when unplaced, got (%d,%v)", got, ok)
	}
	// no ready instances → ok=false
	if _, ok := pickInstance("c-0", "c", nil, map[int32]int{}); ok {
		t.Errorf("expected ok=false with no ready instances")
	}
}

// TestPickInstance_StickyDespiteLoadSkew guards the poison-stream cascade fix:
// while a pipeline's assigned instance is in readyOrdinals it is never migrated,
// even under heavy load skew. With the /ping readiness probe a functionally broken
// stream no longer drops its pod from readyOrdinals, so no migration trigger exists.
// See docs/superpowers/specs/2026-06-11-cluster-poison-stream-cascade-design.md.
func TestPickInstance_StickyDespiteLoadSkew(t *testing.T) {
	// c-0 is the current (ready) instance and heavily loaded; c-1 is idle.
	got, ok := pickInstance("c-0", "c", []int32{0, 1}, map[int32]int{0: 9, 1: 0})
	if !ok || got != 0 {
		t.Fatalf("expected sticky placement on c-0 (no migration), got ordinal=%d ok=%v", got, ok)
	}
}
