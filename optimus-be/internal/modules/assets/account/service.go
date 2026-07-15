package account

import (
	"context"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/assets/errs"
	"optimus-be/internal/modules/audit"
)

var regionRegex = regexp.MustCompile(`^[a-z]{2}-[a-z]+-\d$`)

type CloudKeyExistenceChecker interface {
	Exists(ctx context.Context, id uint64) (bool, error)
}

type AuditRecorder interface {
	Record(ctx context.Context, event audit.Event) error
}

type Service struct {
	repo  *Repo
	audit AuditRecorder
	cks   CloudKeyExistenceChecker
}

func NewService(repo *Repo, recorder AuditRecorder, checker CloudKeyExistenceChecker) *Service {
	return &Service{repo: repo, audit: recorder, cks: checker}
}

func (s *Service) Create(ctx context.Context, actorID uint64, ip, userAgent string, req CreateRequest) (*Detail, error) {
	name, err := normalizeName(req.Name)
	if err != nil {
		return nil, err
	}
	if req.Provider != "aws" {
		return nil, apperr.New(errs.CodeAssetsProviderUnsupported, errs.KeyProviderUnsupported, "only aws is supported")
	}
	if err := validateRegions(req.EnabledRegions); err != nil {
		return nil, err
	}
	exists, err := s.cks.Exists(ctx, req.CloudKeyID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, apperr.New(errs.CodeAssetsCloudKeyNotFound, errs.KeyCloudKeyNotFound, "cloud key not found")
	}
	conflict, err := s.repo.FindNameAlive(ctx, name, 0)
	if err != nil {
		return nil, err
	}
	if conflict {
		return nil, apperr.New(errs.CodeAssetsCloudAccountNameConflict, errs.KeyCloudAccountNameConflict, "cloud account name already exists")
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	row := &models.CloudAccount{
		Name:           name,
		Provider:       req.Provider,
		CloudKeyID:     req.CloudKeyID,
		EnabledRegions: models.StringArray(req.EnabledRegions),
		Enabled:        enabled,
		Description:    req.Description,
	}
	if err := s.repo.Create(ctx, row); err != nil {
		return nil, err
	}
	s.writeAudit(ctx, actorIDOrNil(actorID), "assets.cloud_account.create", row.ID, ip, userAgent, map[string]any{
		"name":        row.Name,
		"provider":    row.Provider,
		"cloudkey_id": row.CloudKeyID,
		"regions":     []string(row.EnabledRegions),
	})
	return s.detail(ctx, row.ID)
}

func (s *Service) Get(ctx context.Context, id uint64) (*Detail, error) {
	return s.detail(ctx, id)
}

func (s *Service) Update(ctx context.Context, actorID uint64, ip, userAgent string, id uint64, req UpdateRequest) (*Detail, error) {
	if req.Name != nil {
		name, err := normalizeName(*req.Name)
		if err != nil {
			return nil, err
		}
		req.Name = &name
	}
	fields := make(map[string]any)
	changed := make([]string, 0, 4)
	removedRegions := make([]string, 0)
	if req.EnabledRegions != nil {
		if err := validateRegions(req.EnabledRegions); err != nil {
			return nil, err
		}
	}
	regionsChanged := false
	err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		row, err := s.repo.FindByIDForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if req.Name != nil && *req.Name != row.Name {
			conflict, err := s.repo.FindNameAliveTx(ctx, tx, *req.Name, row.ID)
			if err != nil {
				return err
			}
			if conflict {
				return apperr.New(errs.CodeAssetsCloudAccountNameConflict, errs.KeyCloudAccountNameConflict, "cloud account name already exists")
			}
			fields["name"] = *req.Name
			changed = append(changed, "name")
		}
		regionsChanged = req.EnabledRegions != nil && !slices.Equal(req.EnabledRegions, []string(row.EnabledRegions))
		if regionsChanged {
			newRegions := make(map[string]struct{}, len(req.EnabledRegions))
			for _, region := range req.EnabledRegions {
				newRegions[region] = struct{}{}
			}
			for _, region := range row.EnabledRegions {
				if _, ok := newRegions[region]; !ok {
					removedRegions = append(removedRegions, region)
				}
			}
			fields["enabled_regions"] = models.StringArray(req.EnabledRegions)
			changed = append(changed, "enabled_regions")
		}
		if req.Enabled != nil && *req.Enabled != row.Enabled {
			fields["enabled"] = *req.Enabled
			changed = append(changed, "enabled")
		}
		if req.Description != nil && *req.Description != row.Description {
			fields["description"] = *req.Description
			changed = append(changed, "description")
		}
		if len(fields) == 0 {
			return nil
		}
		rows, err := s.repo.UpdateTx(ctx, tx, id, fields)
		if err != nil {
			return err
		}
		if rows != 1 {
			return accountNotFoundError()
		}
		if len(removedRegions) > 0 {
			_, err = s.repo.CascadeSoftDeleteResources(ctx, tx, id, removedRegions)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return s.detail(ctx, id)
	}
	sort.Strings(changed)
	sort.Strings(removedRegions)
	payload := map[string]any{"changed_fields": changed}
	if regionsChanged {
		payload["regions"] = append([]string(nil), req.EnabledRegions...)
		payload["regions_removed"] = removedRegions
	}
	s.writeAudit(ctx, actorIDOrNil(actorID), "assets.cloud_account.update", id, ip, userAgent, payload)
	return s.detail(ctx, id)
}

func (s *Service) Delete(ctx context.Context, actorID uint64, ip, userAgent string, id uint64) (int64, error) {
	var row *models.CloudAccount
	var cascaded int64
	err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		var err error
		row, err = s.repo.FindByIDForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		count, err := s.repo.CascadeSoftDeleteResources(ctx, tx, id, nil)
		if err != nil {
			return err
		}
		cascaded = count
		rows, err := s.repo.SoftDeleteTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if rows != 1 {
			return accountNotFoundError()
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	s.writeAudit(ctx, actorIDOrNil(actorID), "assets.cloud_account.delete", id, ip, userAgent, map[string]any{
		"name":                     row.Name,
		"cascaded_resources_count": cascaded,
	})
	return cascaded, nil
}

func accountNotFoundError() error {
	return apperr.New(errs.CodeAssetsCloudAccountNotFound, errs.KeyCloudAccountNotFound, "cloud account not found")
}

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, error) {
	items, total, err := s.repo.List(ctx, query)
	if err != nil {
		return nil, err
	}
	cloudKeyIDs := make([]uint64, 0, len(items))
	for i := range items {
		cloudKeyIDs = append(cloudKeyIDs, items[i].CloudKeyID)
	}
	names, err := s.repo.CloudKeyNames(ctx, cloudKeyIDs)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].CloudKeyName = names[items[i].CloudKeyID]
	}
	return &ListResponse{Items: items, Total: total}, nil
}

func (s *Service) RecordSyncTrigger(ctx context.Context, actor *uint64, ip, userAgent string, accountID uint64) {
	row, _ := s.repo.FindByID(ctx, accountID)
	regions := []string{}
	if row != nil {
		regions = append(regions, row.EnabledRegions...)
	}
	s.writeAudit(ctx, actor, "assets.cloud_account.sync_trigger", accountID, ip, userAgent, map[string]any{
		"regions": regions,
		"trigger": "manual",
	})
}

func (s *Service) detail(ctx context.Context, id uint64) (*Detail, error) {
	row, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	names, err := s.repo.CloudKeyNames(ctx, []uint64{row.CloudKeyID})
	if err != nil {
		return nil, err
	}
	latest, err := s.repo.latestSyncRuns(ctx, []uint64{id})
	if err != nil {
		return nil, err
	}
	sync := latest[id]
	detail := toDetail(*row, names[row.CloudKeyID], sync.startedAt, sync.status)
	return &detail, nil
}

func validateRegions(regions []string) error {
	if len(regions) == 0 {
		return apperr.New(errs.CodeAssetsRegionInvalid, errs.KeyRegionInvalid, "at least one region is required")
	}
	seen := make(map[string]struct{}, len(regions))
	for _, region := range regions {
		if !regionRegex.MatchString(region) {
			return apperr.New(errs.CodeAssetsRegionInvalid, errs.KeyRegionInvalid, "invalid AWS region")
		}
		if _, exists := seen[region]; exists {
			return apperr.New(errs.CodeAssetsRegionInvalid, errs.KeyRegionInvalid, "duplicate AWS region")
		}
		seen[region] = struct{}{}
	}
	return nil
}

func normalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", apperr.New(apperr.CodeValidation, "common.validation", "name is required")
	}
	return name, nil
}

func (s *Service) writeAudit(ctx context.Context, actor *uint64, action string, id uint64, ip, userAgent string, payload map[string]any) {
	_ = s.audit.Record(ctx, audit.Event{
		UserID: actor, Action: action, TargetType: "cloud_account",
		TargetID: strconv.FormatUint(id, 10), Payload: payload,
		IP: ip, UserAgent: userAgent,
	})
}

func actorIDOrNil(id uint64) *uint64 {
	if id == 0 {
		return nil
	}
	return &id
}
