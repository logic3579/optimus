//go:build dbtest

package rbac_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"optimus-be/internal/infra/db"
	"optimus-be/internal/infra/permissions"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/rbac"
	"optimus-be/internal/modules/user"
	"optimus-be/internal/seed"
)

func TestMeService_GetUser(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	u := &models.User{Username: "alice", Email: "a@x", PasswordHash: "x", Status: "enabled", DisplayName: "Alice"}
	require.NoError(t, gdb.Create(u).Error)

	svc := rbac.NewMeService(gdb, rbac.NewPermissionCache(gdb, time.Minute), nil)
	dto, err := svc.GetUser(context.Background(), u.ID)
	require.NoError(t, err)
	require.Equal(t, u.ID, dto.ID)
	require.Equal(t, "alice", dto.Username)
	require.Equal(t, "Alice", dto.DisplayName)
}

func TestMeService_GetUser_NotFound(t *testing.T) {
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	defer teardown()
	svc := rbac.NewMeService(gdb, rbac.NewPermissionCache(gdb, 0), nil)
	_, err := svc.GetUser(context.Background(), 999999)
	require.Error(t, err)
}

// meServiceFixture bundles the dependencies needed by MeService write-path
// tests so individual tests stay readable. Each fixture owns its own postgres
// schema (via dockertest) — call cleanup to drop it.
type meServiceFixture struct {
	ctx     context.Context
	gdb     *gorm.DB
	svc     *rbac.MeService
	userSvc *user.Service
	cleanup func()
}

// setupMeServiceTest spins up a Postgres schema, registers permissions,
// seeds builtin admin, and wires a MeService backed by a real user.Service so
// the /me write adapters can be exercised end-to-end against the database.
func setupMeServiceTest(t *testing.T) *meServiceFixture {
	t.Helper()
	gdb, teardown := db.StartTestPostgres(t, filepath.Join("..", "..", "..", "migrations"))
	ctx := context.Background()
	_, err := permissions.Register(ctx, gdb, permissions.All)
	require.NoError(t, err)
	_, err = seed.Run(ctx, gdb, seed.Options{AdminUsername: "admin", AdminEmail: "a@x", BcryptCost: 4})
	require.NoError(t, err)

	cache := rbac.NewPermissionCache(gdb, time.Minute)
	rec := audit.NewRecorder(gdb)
	userSvc := user.NewService(user.NewRepo(gdb), cache, rec, user.ServiceOptions{BcryptCost: 4, AdminUsername: "admin"})
	svc := rbac.NewMeService(gdb, cache, userSvc)
	return &meServiceFixture{ctx: ctx, gdb: gdb, svc: svc, userSvc: userSvc, cleanup: teardown}
}

func (d *meServiceFixture) seedUser(t *testing.T, username, email string) uint64 {
	t.Helper()
	return d.seedUserWithPassword(t, username, email, "Pass1234")
}

func (d *meServiceFixture) seedUserWithPassword(t *testing.T, username, email, password string) uint64 {
	t.Helper()
	out, err := d.userSvc.Create(d.ctx, 1, "127.0.0.1", "go-test", user.CreateRequest{
		Username: username, Email: email, Password: password,
	})
	require.NoError(t, err)
	require.NotZero(t, out.ID)
	return out.ID
}

func TestMeService_UpdateMe_OK(t *testing.T) {
	d := setupMeServiceTest(t)
	defer d.cleanup()

	uid := d.seedUser(t, "alice", "alice@example.com")
	email := "alice2@example.com"
	display := "Alice Cooper"
	dto, err := d.svc.UpdateMe(d.ctx, uid, "1.1.1.1", "ua", rbac.UpdateMeRequest{Email: &email, DisplayName: &display})
	require.NoError(t, err)
	require.Equal(t, "alice2@example.com", dto.Email)
	require.Equal(t, "Alice Cooper", dto.DisplayName)
}

func TestMeService_ChangeMyPassword_OK(t *testing.T) {
	d := setupMeServiceTest(t)
	defer d.cleanup()

	uid := d.seedUserWithPassword(t, "alice", "alice@example.com", "oldpass1234")
	require.NoError(t, d.svc.ChangeMyPassword(d.ctx, uid, "1.1.1.1", "ua", "oldpass1234", "newpass5678"))
}

func TestMeService_ChangeMyPassword_WrongOld(t *testing.T) {
	d := setupMeServiceTest(t)
	defer d.cleanup()

	uid := d.seedUserWithPassword(t, "alice", "alice@example.com", "rightpass00")
	err := d.svc.ChangeMyPassword(d.ctx, uid, "1.1.1.1", "ua", "wrongpass", "newpass5678")
	require.Error(t, err)
}
