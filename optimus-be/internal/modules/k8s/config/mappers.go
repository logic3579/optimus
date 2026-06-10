package config

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
)

// toSummary projects a ConfigMap into the list-row DTO. Keys from Data and
// BinaryData are merged and sorted so the output is deterministic regardless
// of map iteration order (important for tests and for stable FE rendering).
func toSummary(c corev1.ConfigMap) MapSummary {
	keys := make([]string, 0, len(c.Data)+len(c.BinaryData))
	for k := range c.Data {
		keys = append(keys, k)
	}
	for k := range c.BinaryData {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return MapSummary{
		Name:      c.Name,
		Namespace: c.Namespace,
		DataKeys:  keys,
		DataCount: len(keys),
		Labels:    c.Labels,
		Age:       c.CreationTimestamp.Time,
	}
}

// toDetail extends the summary with the full string Data and a sorted list
// of BinaryData keys (payload deliberately omitted — see dto.go).
func toDetail(c corev1.ConfigMap) MapDetail {
	binKeys := make([]string, 0, len(c.BinaryData))
	for k := range c.BinaryData {
		binKeys = append(binKeys, k)
	}
	sort.Strings(binKeys)
	return MapDetail{
		MapSummary: toSummary(c),
		Data:       c.Data,
		BinaryKeys: binKeys,
	}
}
