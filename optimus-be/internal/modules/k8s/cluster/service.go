package cluster

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/version"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/credentials"
)

// VersionProbe is the minimal surface of k8s.io/client-go/discovery's
// ServerVersionInterface — kept narrow on purpose so tests don't need the
// full discovery client, and so that importing this package doesn't force
// the heavy managedfields/openapi transitive dependency tree (which clashes
// with the structured-merge-diff/v6 + kube-openapi pinning surface elsewhere
// in the module graph). Task 7's Factory adapts a real DiscoveryInterface
// into one of these.
type VersionProbe interface {
	ServerVersion() (*version.Info, error)
}

// Prober is the abstraction the Service uses to talk to apiserver during Ping.
// In production it's wired to client.Factory; tests inject a fake.
type Prober interface {
	Discover(ctx context.Context, clusterID uint64, purpose string) (VersionProbe, error)
}

// AppsApplicationCounter is the narrow seam k8s/cluster.Delete uses to refuse
// deletion when applications still reference the cluster. Satisfied by
// apps/application.Repo (or any Counter that exposes CountByClusterID);
// wired post-construction by main.go via SetAppsCounter so this package
// stays free of any apps/* import (which would create an import cycle, as
// apps/application's Repo Get uses Preload("Cluster") and depends on
// models.Cluster).
type AppsApplicationCounter interface {
	CountByClusterID(ctx context.Context, clusterID uint64) (int, error)
}

// DiscoveryFunc is a tiny adapter so anonymous closures satisfy Prober.
type DiscoveryFunc func(ctx context.Context, clusterID uint64, purpose string) (VersionProbe, error)

// Discover implements Prober for DiscoveryFunc.
func (f DiscoveryFunc) Discover(ctx context.Context, id uint64, purpose string) (VersionProbe, error) {
	return f(ctx, id, purpose)
}

type Service struct {
	repo        *Repo
	consumer    credentials.Consumer    // used by Create/Update to fetch + validate the kubeconfig YAML
	prober      Prober                  // nil-safe: Ping returns ok=false with message if nil
	audit       *audit.Recorder
	appsCounter AppsApplicationCounter // nil-safe: Delete skips the pre-check if unwired
}

func NewService(repo *Repo, consumer credentials.Consumer, prober Prober, rec *audit.Recorder) *Service {
	return &Service{repo: repo, consumer: consumer, prober: prober, audit: rec}
}

// SetAppsCounter wires the application-count seam post-construction. nil is
// allowed (the Delete pre-check is then skipped) so the k8s module can be
// brought up before apps/application is constructed in main.go.
func (s *Service) SetAppsCounter(c AppsApplicationCounter) { s.appsCounter = c }

// Repo lets callers (P1 inuse helper, tests) reach the underlying DB.
func (s *Service) Repo() *Repo { return s.repo }

// --- queries ---------------------------------------------------------------

func (s *Service) List(ctx context.Context, q ListQuery) (*ListResponse, error) {
	rows, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSummary(r))
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 20
	}
	return &ListResponse{Items: out, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

func (s *Service) Get(ctx context.Context, id uint64) (*Detail, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "k8s.cluster.not_found", "cluster not found")
		}
		return nil, err
	}
	d := Detail(toSummary(*m))
	return &d, nil
}

// --- mutations -------------------------------------------------------------

func (s *Service) Create(ctx context.Context, actorID uint64, ip, ua string, req CreateRequest) (*Detail, error) {
	// 1. Pull kubeconfig YAML through Consumer and validate context + auth.
	if err := s.validateRef(ctx, actorID, req.KubeconfigID, req.Context); err != nil {
		return nil, err
	}
	// 2. Name uniqueness.
	if _, err := s.repo.FindByName(ctx, strings.TrimSpace(req.Name)); err == nil {
		return nil, apperr.New(apperr.CodeConflict, "k8s.cluster.name_taken", "cluster name already exists")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	m := &models.Cluster{
		Name:         strings.TrimSpace(req.Name),
		KubeconfigID: req.KubeconfigID,
		Context:      req.Context,
		Description:  req.Description,
		Tags:         req.Tags,
	}
	if m.Tags == nil {
		m.Tags = []string{}
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.writeAudit(ctx, ptrIfNonZero(actorID), "k8s.cluster.create", m.ID, ip, ua, map[string]any{
		"name":          m.Name,
		"kubeconfig_id": m.KubeconfigID,
		"context":       m.Context,
		"tags":          []string(m.Tags),
	})
	return s.Get(ctx, m.ID)
}

func (s *Service) Update(ctx context.Context, actorID uint64, ip, ua string, id uint64, req UpdateRequest) (*Detail, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "k8s.cluster.not_found", "cluster not found")
		}
		return nil, err
	}

	fields := map[string]any{}
	var changed []string
	finalName := m.Name
	finalKubeconfigID := m.KubeconfigID
	finalContext := m.Context

	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if n != m.Name {
			if _, err := s.repo.FindByName(ctx, n); err == nil {
				return nil, apperr.New(apperr.CodeConflict, "k8s.cluster.name_taken", "cluster name already exists")
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
			fields["name"] = n
			changed = append(changed, "name")
			finalName = n
		}
	}
	if req.KubeconfigID != nil && *req.KubeconfigID != m.KubeconfigID {
		finalKubeconfigID = *req.KubeconfigID
		fields["kubeconfig_id"] = finalKubeconfigID
		changed = append(changed, "kubeconfig_id")
	}
	if req.Context != nil && *req.Context != m.Context {
		finalContext = *req.Context
		fields["context"] = finalContext
		changed = append(changed, "context")
	}
	if req.Description != nil && *req.Description != m.Description {
		fields["description"] = *req.Description
		changed = append(changed, "description")
	}
	if req.Tags != nil {
		tags := *req.Tags
		if tags == nil {
			tags = []string{}
		}
		// Wrap as JSONSlice so GORM goes through Value() and writes proper
		// JSONB rather than a Go-syntax string.
		fields["tags"] = datatypes.NewJSONSlice[string](tags)
		changed = append(changed, "tags")
	}

	// If kubeconfig or context changed, re-validate the pair.
	if _, ok := fields["kubeconfig_id"]; ok {
		if err := s.validateRef(ctx, actorID, finalKubeconfigID, finalContext); err != nil {
			return nil, err
		}
	} else if _, ok := fields["context"]; ok {
		if err := s.validateRef(ctx, actorID, finalKubeconfigID, finalContext); err != nil {
			return nil, err
		}
	}

	if len(fields) > 0 {
		if err := s.repo.Update(ctx, id, fields); err != nil {
			return nil, err
		}
	}
	if len(changed) > 0 {
		s.writeAudit(ctx, ptrIfNonZero(actorID), "k8s.cluster.update", id, ip, ua, map[string]any{
			"name":           finalName,
			"changed_fields": changed,
		})
	}
	return s.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, actorID uint64, ip, ua string, id uint64) error {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.New(apperr.CodeNotFound, "k8s.cluster.not_found", "cluster not found")
		}
		return err
	}
	// P3 pre-check: refuse delete while any application still references this
	// cluster. Seam is wired by main.go (cluster pkg never imports apps/*).
	if s.appsCounter != nil {
		n, cerr := s.appsCounter.CountByClusterID(ctx, id)
		if cerr != nil {
			return cerr
		}
		if n > 0 {
			return apperr.New(apperr.CodeAppsApplicationInUse,
				"k8s.cluster.in_use_by_apps",
				fmt.Sprintf("%d application(s) still reference this cluster", n))
		}
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.writeAudit(ctx, ptrIfNonZero(actorID), "k8s.cluster.delete", id, ip, ua, map[string]any{
		"name": m.Name,
	})
	return nil
}

func (s *Service) Ping(ctx context.Context, actorID uint64, ip, ua string, id uint64) (*PingResult, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "k8s.cluster.not_found", "cluster not found")
		}
		return nil, err
	}
	res := &PingResult{}
	if s.prober == nil {
		res.Message = "prober not configured"
	} else {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		disc, derr := s.prober.Discover(pingCtx, id, "k8s.health.ping")
		switch {
		case derr != nil:
			res.Message = derr.Error()
		case disc == nil:
			res.Message = "prober returned nil discovery client"
		default:
			v, vErr := disc.ServerVersion()
			switch {
			case vErr != nil:
				res.Message = vErr.Error()
			case v == nil:
				res.Message = "empty server version"
			default:
				res.OK = true
				res.ServerVersion = v.GitVersion
			}
		}
	}
	// Persist health snapshot (best-effort).
	_ = s.repo.UpdateHealth(ctx, id, res.OK, res.Message)
	s.writeAudit(ctx, ptrIfNonZero(actorID), "k8s.cluster.ping", id, ip, ua, map[string]any{
		"name":           m.Name,
		"ok":             res.OK,
		"server_version": res.ServerVersion,
		"error":          res.Message,
	})
	return res, nil
}

// --- helpers ---------------------------------------------------------------

// validateRef pulls the referenced kubeconfig through the Consumer seam (which
// also writes the consume audit row), then validates that the named context
// exists and that no AuthInfo uses an exec or auth-provider plugin. This is
// the user-visible reject point on Create/Update; Task 7's Factory will repeat
// the check at run-time as defense in depth.
func (s *Service) validateRef(ctx context.Context, actorID uint64, kubeconfigID uint64, contextName string) error {
	if actorID != 0 {
		ctx = credentials.WithActor(ctx, actorID)
	}
	rec, err := s.consumer.GetKubeconfig(ctx, kubeconfigID, "k8s.cluster.validate")
	if err != nil {
		return err // 40401 / 40002 / etc from P1
	}
	return ValidateContextAndAuth(rec.YAML, contextName)
}

func (s *Service) writeAudit(ctx context.Context, actor *uint64, action string, id uint64, ip, ua string, payload map[string]any) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID:     actor,
		Action:     action,
		TargetType: "k8s.cluster",
		TargetID:   strconv.FormatUint(id, 10),
		Payload:    payload,
		IP:         ip,
		UserAgent:  ua,
	})
}

func ptrIfNonZero(v uint64) *uint64 {
	if v == 0 {
		return nil
	}
	return &v
}

func toSummary(m models.Cluster) Summary {
	out := Summary{
		ID:            m.ID,
		Name:          m.Name,
		KubeconfigID:  m.KubeconfigID,
		Context:       m.Context,
		Description:   m.Description,
		Tags:          []string(m.Tags),
		LastHealthAt:  m.LastHealthAt,
		LastHealthOK:  m.LastHealthOK,
		LastHealthMsg: m.LastHealthMsg,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
	if m.Kubeconfig != nil {
		out.KubeconfigName = m.Kubeconfig.Name
	}
	if out.Tags == nil {
		out.Tags = []string{}
	}
	return out
}
