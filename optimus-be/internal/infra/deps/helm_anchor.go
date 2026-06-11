//go:build helm_anchor

// Package deps holds blank-import anchors that keep `go mod tidy` from
// stripping direct dependencies before downstream packages import them.
//
// helm_anchor build tag is never set; this file exists ONLY so the Go
// module graph treats helm.sh/helm/v3 as a direct dependency until the
// real consumers (apps/release service + helmclient.Factory) land in P3
// task T9/T10. At that point this file can be deleted.
package deps

import (
	// helm.sh/helm/v3 is pinned to v3.15.4 to preserve the
	// k8s.io/client-go v0.30.14 invariant established by P2. Helm v3.16.x
	// transitively upgrades client-go to v0.31.x, which would push the
	// go directive past 1.25 and break Dockerfile + CI.
	_ "helm.sh/helm/v3/pkg/action"
)
