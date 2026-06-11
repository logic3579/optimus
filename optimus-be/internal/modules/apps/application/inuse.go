package application

import "context"

// Counter is the seam exposed to other packages so they can do FK pre-checks
// (apps/repo.Delete and k8s/cluster.Delete both reach in here). The concrete
// implementation is the Repo — main.go wires it post-construction to break
// the import cycle (apps/repo and k8s/cluster never import apps/application).
type Counter interface {
	CountByClusterID(ctx context.Context, clusterID uint64) (int, error)
	CountByChartRepoID(ctx context.Context, repoID uint64) (int, error)
}

// Compile-time guarantee Repo satisfies Counter.
var _ Counter = (*Repo)(nil)
