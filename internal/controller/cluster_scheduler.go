/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	"sort"
	"strconv"
	"strings"
)

// ordinalFromPodName extracts the StatefulSet ordinal from a pod name of the
// form "<cluster>-<ordinal>". Returns ok=false if the name does not match.
func ordinalFromPodName(podName, clusterName string) (int32, bool) {
	prefix := clusterName + "-"
	if !strings.HasPrefix(podName, prefix) {
		return 0, false
	}
	n, err := strconv.Atoi(podName[len(prefix):])
	if err != nil || n < 0 {
		return 0, false
	}
	return int32(n), true
}

// pickInstance chooses the instance for a pipeline: its current ordinal if that
// instance is still ready (sticky placement — avoids spurious migration on plain
// reconciles), otherwise the least-loaded ready instance. currentInstance is the
// pipeline's status.assignedInstance (empty when unplaced).
func pickInstance(currentInstance, clusterName string, readyOrdinals []int32, loadByOrdinal map[int32]int) (int32, bool) {
	if cur, ok := ordinalFromPodName(currentInstance, clusterName); ok {
		for _, o := range readyOrdinals {
			if o == cur {
				return cur, true
			}
		}
	}
	return leastLoadedInstance(readyOrdinals, loadByOrdinal)
}

// leastLoadedInstance picks the ready ordinal with the fewest placed pipelines.
// Ties resolve to the lowest ordinal. Returns ok=false when no instance is ready.
func leastLoadedInstance(readyOrdinals []int32, loadByOrdinal map[int32]int) (int32, bool) {
	if len(readyOrdinals) == 0 {
		return 0, false
	}
	sorted := append([]int32(nil), readyOrdinals...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	best := sorted[0]
	bestLoad := loadByOrdinal[best]
	for _, o := range sorted[1:] {
		if loadByOrdinal[o] < bestLoad {
			best, bestLoad = o, loadByOrdinal[o]
		}
	}
	return best, true
}
