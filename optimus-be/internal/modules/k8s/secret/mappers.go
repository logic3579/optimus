package secret

import (
	"encoding/base64"
	"sort"
	"unicode/utf8"

	corev1 "k8s.io/api/core/v1"
)

// toSummary projects a Secret into the list/get row DTO. Only the key names
// and metadata are surfaced — the raw byte values are never copied into the
// output. Keys are sorted so the output is deterministic regardless of map
// iteration order (important for tests and for stable FE rendering).
func toSummary(s corev1.Secret) Summary {
	keys := make([]string, 0, len(s.Data))
	for k := range s.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return Summary{
		Name:      s.Name,
		Namespace: s.Namespace,
		Type:      string(s.Type),
		DataKeys:  keys,
		DataCount: len(keys),
		Labels:    s.Labels,
		Age:       s.CreationTimestamp.Time,
	}
}

// toData projects the Secret payload into the reveal envelope. Each value is
// inspected: if it is valid UTF-8 it is returned as a plain string so the FE
// can show it directly; otherwise it is wrapped in
// {"value": "<base64>", "base64": true} so the FE can render a binary
// placeholder / download link without us guessing an encoding.
func toData(s corev1.Secret) DataResponse {
	out := DataResponse{Data: make(map[string]any, len(s.Data))}
	for k, v := range s.Data {
		if utf8.Valid(v) {
			out.Data[k] = string(v)
		} else {
			out.Data[k] = map[string]any{
				"value":  base64.StdEncoding.EncodeToString(v),
				"base64": true,
			}
		}
	}
	return out
}
