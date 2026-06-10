// Package credentials is the entry point for downstream sub-projects (P2/P4/P5/P6)
// that need to read decrypted credential material. The exported Consumer interface
// is the SOLE public API — downstream callers must not import the sshkey /
// kubeconfig / cloudkey sub-packages directly.
//
// Permission semantics: this seam does NOT enforce credentials:*:use. Downstream
// packages enforce their own feature-specific RBAC (e.g., k8s:exec:write); the
// :use code is registered for role-management visibility but P1 itself does not
// gate calls on it.
package credentials

import (
	"context"

	"optimus-be/internal/infra/middleware"
	"optimus-be/internal/modules/credentials/cloudkey"
	"optimus-be/internal/modules/credentials/kubeconfig"
	"optimus-be/internal/modules/credentials/sshkey"
)

// SSHKey is the decrypted shape returned by Consumer.GetSSHKey.
type SSHKey struct {
	Name       string
	Username   string
	PrivateKey []byte
	Passphrase []byte // nil if not set
}

// Kubeconfig is the decrypted shape returned by Consumer.GetKubeconfig.
type Kubeconfig struct {
	Name             string
	DefaultNamespace string
	YAML             []byte
}

// CloudKey is the decrypted shape returned by Consumer.GetCloudKey.
type CloudKey struct {
	Name            string
	Provider        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
}

// Consumer is the seam used by downstream sub-projects. `purpose` is a free-form
// caller-supplied string recorded in audit; for system callers (ctx has no user_id),
// it must start with "system:".
type Consumer interface {
	GetSSHKey(ctx context.Context, id uint64, purpose string) (*SSHKey, error)
	GetKubeconfig(ctx context.Context, id uint64, purpose string) (*Kubeconfig, error)
	GetCloudKey(ctx context.Context, id uint64, purpose string) (*CloudKey, error)
}

// NewConsumer wires a Consumer over the three feature services. Callers obtain
// services from credentials.New (see module.go).
func NewConsumer(ssh *sshkey.Service, kc *kubeconfig.Service, ck *cloudkey.Service) Consumer {
	return &consumer{ssh: ssh, kc: kc, ck: ck}
}

type consumer struct {
	ssh *sshkey.Service
	kc  *kubeconfig.Service
	ck  *cloudkey.Service
}

func (c *consumer) GetSSHKey(ctx context.Context, id uint64, purpose string) (*SSHKey, error) {
	rec, err := c.ssh.Consume(ctx, actorFromCtx(ctx), id, purpose)
	if err != nil {
		return nil, err
	}
	return &SSHKey{
		Name:       rec.Name,
		Username:   rec.Username,
		PrivateKey: rec.PrivateKey,
		Passphrase: rec.Passphrase,
	}, nil
}

func (c *consumer) GetKubeconfig(ctx context.Context, id uint64, purpose string) (*Kubeconfig, error) {
	rec, err := c.kc.Consume(ctx, actorFromCtx(ctx), id, purpose)
	if err != nil {
		return nil, err
	}
	return &Kubeconfig{
		Name:             rec.Name,
		DefaultNamespace: rec.DefaultNamespace,
		YAML:             rec.YAML,
	}, nil
}

func (c *consumer) GetCloudKey(ctx context.Context, id uint64, purpose string) (*CloudKey, error) {
	rec, err := c.ck.Consume(ctx, actorFromCtx(ctx), id, purpose)
	if err != nil {
		return nil, err
	}
	return &CloudKey{
		Name:            rec.Name,
		Provider:        rec.Provider,
		Region:          rec.Region,
		AccessKeyID:     rec.AccessKeyID,
		SecretAccessKey: rec.SecretAccessKey,
	}, nil
}

// actorFromCtx extracts the actor user_id from a context.Context if present.
// Returns nil for system callers (no actor set in context).
//
// Note: gin's c.Request.Context() does NOT carry the values set via gin's
// c.Set(). HTTP entry points that use the Consumer seam must propagate the
// user_id explicitly via context.WithValue before calling — see WithActor.
func actorFromCtx(ctx context.Context) *uint64 {
	if ctx == nil {
		return nil
	}
	v := ctx.Value(middleware.CtxKeyUserID)
	if v == nil {
		return nil
	}
	if id, ok := v.(uint64); ok && id != 0 {
		return &id
	}
	return nil
}

// WithActor returns a new context carrying actor as the user_id, so a
// downstream service that obtains its ctx from non-HTTP code (cron, queue
// worker) can still drive audit attribution through the Consumer seam.
func WithActor(ctx context.Context, actor uint64) context.Context {
	return context.WithValue(ctx, middleware.CtxKeyUserID, actor)
}
