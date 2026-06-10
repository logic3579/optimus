// Package k8s is the composition root for the P2 Kubernetes management
// surface. It wires the cluster CRUD service, the per-request clientset
// factory, and the read-only verticals (workload / network / config /
// secret / clusterscoped / yaml / log) into a single Module value with
// one MountRoutes entrypoint.
package k8s

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials"
	"optimus-be/internal/modules/k8s/client"
	"optimus-be/internal/modules/k8s/cluster"
	"optimus-be/internal/modules/k8s/clusterscoped"
	"optimus-be/internal/modules/k8s/config"
	logmod "optimus-be/internal/modules/k8s/log"
	"optimus-be/internal/modules/k8s/network"
	"optimus-be/internal/modules/k8s/secret"
	"optimus-be/internal/modules/k8s/workload"
	yamlmod "optimus-be/internal/modules/k8s/yaml"
	"optimus-be/internal/modules/rbac"
)

// Module bundles every k8s sub-service + handler so cmd/server/main.go only
// needs to call New + MountRoutes. Exported fields (Cluster) are the Go-only
// seams used by tests or future sub-projects (the rest stay unexported).
type Module struct {
	// Cluster exposes the CRUD service so future P3+ code can list / lookup
	// clusters without going through HTTP. Currently referenced by main.go
	// only to satisfy vet/staticcheck (it is "live" via the routes).
	Cluster *cluster.Service

	factory        *client.Factory
	clusterHandler *cluster.Handler
	csH            *clusterscoped.Handler
	wlH            *workload.Handler
	netH           *network.Handler
	cfgH           *config.Handler
	secH           *secret.Handler
	yamlH          *yamlmod.Handler
	logH           *logmod.Handler
}

// New constructs the full Module graph. `consumer` comes from the P1
// credentials module (the single Go-only seam for credential reads);
// `rec` is the shared audit recorder so cluster mutations land in the
// same sink as /users and /me writes; `cache` is the shared rbac cache
// (only the yaml handler holds onto it for handler-internal permission
// dispatch — every other route is gated by middleware.RequirePermission
// in MountRoutes below).
func New(db *gorm.DB, consumer credentials.Consumer, rec *audit.Recorder, cache *rbac.PermissionCache) *Module {
	clRepo := cluster.NewRepo(db)
	factory := client.NewFactory(consumer, client.NewRepoAdapter(clRepo))

	// One concrete *client.Factory satisfies every vertical's local
	// Clientsetter / StreamClientsetter interface via structural typing.
	clSvc := cluster.NewService(clRepo, consumer, cluster.DiscoveryFunc(factory.Discover), rec)

	return &Module{
		Cluster:        clSvc,
		factory:        factory,
		clusterHandler: cluster.NewHandler(clSvc),
		csH:            clusterscoped.NewHandler(clusterscoped.NewService(factory)),
		wlH:            workload.NewHandler(workload.NewService(factory)),
		netH:           network.NewHandler(network.NewService(factory)),
		cfgH:           config.NewHandler(config.NewService(factory)),
		secH:           secret.NewHandler(secret.NewService(factory)),
		yamlH:          yamlmod.NewHandler(factory, cache),
		logH:           logmod.NewHandler(factory),
	}
}

// MountRoutes registers all 21 k8s routes under `protected` (which must
// already be JWT-gated). Permission gating happens via nested sub-groups
// with middleware.RequirePermission — see cmd/server/main.go's
// mountUserRoutes for the rationale (variadic args to GET/POST do NOT
// guarantee middleware-before-handler ordering when handlers are
// registered separately; only Group("", mw) does).
//
// The /yaml endpoint is special: it dispatches permission checks
// internally based on the ?kind= query param (workload vs network vs
// config vs secret etc), so no middleware is wrapped around it here.
func (m *Module) MountRoutes(protected *gin.RouterGroup, cache *rbac.PermissionCache) {
	k := protected.Group("/k8s")

	// ---- cluster CRUD ------------------------------------------------------
	cl := k.Group("/clusters")
	rd := cl.Group("", middleware.RequirePermission(cache, "k8s:cluster:read"))
	rd.GET("", m.clusterHandler.HandleList())
	rd.GET("/:id", m.clusterHandler.HandleGet())
	rd.POST("/:id/ping", m.clusterHandler.HandlePing())
	wr := cl.Group("", middleware.RequirePermission(cache, "k8s:cluster:write"))
	wr.POST("", m.clusterHandler.HandleCreate())
	wr.PUT("/:id", m.clusterHandler.HandleUpdate())
	wr.DELETE("/:id", m.clusterHandler.HandleDelete())

	// ---- per-cluster live resources ---------------------------------------
	live := k.Group("/clusters/:id")

	csrd := live.Group("", middleware.RequirePermission(cache, "k8s:cluster_resource:read"))
	csrd.GET("/namespaces", m.csH.ListNamespaces())
	csrd.GET("/nodes", m.csH.ListNodes())
	csrd.GET("/nodes/:name", m.csH.GetNode())
	csrd.GET("/events", m.csH.ListEvents())

	wlrd := live.Group("", middleware.RequirePermission(cache, "k8s:workload:read"))
	wlrd.GET("/workloads/:kind", m.wlH.List())
	wlrd.GET("/workloads/:kind/:ns/:name", m.wlH.Get())

	netrd := live.Group("", middleware.RequirePermission(cache, "k8s:network:read"))
	netrd.GET("/network/:kind", m.netH.List())
	netrd.GET("/network/:kind/:ns/:name", m.netH.Get())

	cfgrd := live.Group("", middleware.RequirePermission(cache, "k8s:config:read"))
	cfgrd.GET("/config/configmaps", m.cfgH.List())
	cfgrd.GET("/config/configmaps/:ns/:name", m.cfgH.Get())

	secrd := live.Group("", middleware.RequirePermission(cache, "k8s:secret:read"))
	secrd.GET("/secrets", m.secH.List())
	secrd.GET("/secrets/:ns/:name", m.secH.Get())
	secreveal := live.Group("", middleware.RequirePermission(cache, "k8s:secret:reveal"))
	secreveal.GET("/secrets/:ns/:name/data", m.secH.Data())

	logrd := live.Group("", middleware.RequirePermission(cache, "k8s:log:read"))
	logrd.GET("/pods/:ns/:name/log", m.logH.Stream())

	// YAML endpoint — permission dispatched inside the handler based on
	// ?kind=, so no middleware is wrapped around the route itself.
	live.GET("/yaml", m.yamlH.Get())
}
