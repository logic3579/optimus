package credentials

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials/cloudkey"
	"optimus-be/internal/modules/credentials/kubeconfig"
	"optimus-be/internal/modules/credentials/sshkey"
	"optimus-be/internal/modules/credentials/vault"
	"optimus-be/internal/modules/rbac"
)

// Module bundles the three feature services and exposes a Consumer for
// downstream sub-projects.
type Module struct {
	SSH        *sshkey.Service
	Kubeconfig *kubeconfig.Service
	CloudKey   *cloudkey.Service
	Consumer   Consumer

	sshHandler *sshkey.Handler
	kcHandler  *kubeconfig.Handler
	ckHandler  *cloudkey.Handler
}

// New constructs the module. cipher must be a real *vault.Cipher (or test fake).
func New(db *gorm.DB, cipher *vault.Cipher, rec *audit.Recorder) *Module {
	ssvc := sshkey.NewService(sshkey.NewRepo(db), cipher, rec)
	ksvc := kubeconfig.NewService(kubeconfig.NewRepo(db), cipher, rec)
	csvc := cloudkey.NewService(cloudkey.NewRepo(db), cipher, rec)
	return &Module{
		SSH:        ssvc,
		Kubeconfig: ksvc,
		CloudKey:   csvc,
		Consumer:   NewConsumer(ssvc, ksvc, csvc),
		sshHandler: sshkey.NewHandler(ssvc),
		kcHandler:  kubeconfig.NewHandler(ksvc),
		ckHandler:  cloudkey.NewHandler(csvc),
	}
}

// crudHandler is the shape every credential-feature handler exposes for
// route mounting. Defined here (not at each sub-package) because the only
// caller is MountRoutes below.
type crudHandler interface {
	HandleList() gin.HandlerFunc
	HandleGet() gin.HandlerFunc
	HandleCreate() gin.HandlerFunc
	HandleUpdate() gin.HandlerFunc
	HandleDelete() gin.HandlerFunc
}

// MountRoutes wires all three CRUD surfaces under /credentials with per-route
// RBAC gates per spec §5.1. Call from cmd/server/main.go inside the protected
// router group (after JWTAuth middleware).
func (m *Module) MountRoutes(protected *gin.RouterGroup, cache *rbac.PermissionCache) {
	mount := func(path, resource string, h crudHandler) {
		g := protected.Group("/credentials/" + path)

		rd := g.Group("", middleware.RequirePermission(cache, "credentials:"+resource+":read"))
		rd.GET("", h.HandleList())
		rd.GET("/:id", h.HandleGet())

		wr := g.Group("", middleware.RequirePermission(cache, "credentials:"+resource+":write"))
		wr.POST("", h.HandleCreate())
		wr.PUT("/:id", h.HandleUpdate())

		del := g.Group("", middleware.RequirePermission(cache, "credentials:"+resource+":delete"))
		del.DELETE("/:id", h.HandleDelete())
	}
	mount("ssh-keys", "ssh_key", m.sshHandler)
	mount("kubeconfigs", "kubeconfig", m.kcHandler)
	mount("cloud-keys", "cloud_key", m.ckHandler)
}
