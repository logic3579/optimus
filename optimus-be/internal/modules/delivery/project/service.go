package project

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/delivery/errs"
)

type ApplicationReader interface {
	GetApplication(ctx context.Context, id uint64) (*Application, error)
}

// ProjectActivity distinguishes ordinary active work from an unknown outcome,
// because the latter requires operator reconciliation before destructive edits.
type ProjectActivity struct {
	Active         bool
	OutcomeUnknown bool
}

type ProjectActivityReader interface {
	ProjectActivity(ctx context.Context, projectID uint64) (ProjectActivity, error)
}

// Recorder deliberately matches audit.Recorder.Record without exposing its DB.
type Recorder interface {
	Record(ctx context.Context, event audit.Event) error
}

type projectRepository interface {
	Transaction(ctx context.Context, fn func(projectRepository) error) error
	ListProjects(ctx context.Context, q ListQuery) ([]models.DeliveryProject, int64, error)
	GetProject(ctx context.Context, id uint64) (*models.DeliveryProject, error)
	LockProject(ctx context.Context, id uint64) (*models.DeliveryProject, error)
	LockApplication(ctx context.Context, applicationID uint64) error
	CreateProject(ctx context.Context, row *models.DeliveryProject) error
	UpdateProject(ctx context.Context, id uint64, fields map[string]any) error
	DeleteProject(ctx context.Context, id uint64) error
	ListEnvironments(ctx context.Context, projectID uint64) ([]models.DeliveryEnvironment, error)
	GetEnvironment(ctx context.Context, projectID, id uint64) (*models.DeliveryEnvironment, error)
	ApplicationBound(ctx context.Context, applicationID uint64) (bool, error)
	CreateEnvironment(ctx context.Context, row *models.DeliveryEnvironment) error
	UpdateEnvironment(ctx context.Context, projectID, id uint64, fields map[string]any) error
	DeleteEnvironment(ctx context.Context, projectID, id uint64) error
	CountActiveEnvironments(ctx context.Context, projectID uint64) (int64, error)
	CountPipelineReferences(ctx context.Context, environmentID uint64) (int64, error)
}

type Service struct {
	repo     projectRepository
	apps     ApplicationReader
	activity ProjectActivityReader
	audit    Recorder
}

func NewService(repo projectRepository, apps ApplicationReader, activity ProjectActivityReader, recorder Recorder) *Service {
	return &Service{repo: repo, apps: apps, activity: activity, audit: recorder}
}

// List is the handler-facing alias for ListProjects.
func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, error) {
	return s.ListProjects(ctx, query)
}

// Get is the handler-facing alias for GetProject.
func (s *Service) Get(ctx context.Context, id uint64) (*ProjectDetail, error) {
	return s.GetProject(ctx, id)
}

func (s *Service) ListProjects(ctx context.Context, query ListQuery) (*ListResponse, error) {
	page, size, _, err := pageValues(query.Page, query.PageSize)
	if err != nil {
		return nil, err
	}
	query.Page, query.PageSize = page, size
	rows, total, err := s.repo.ListProjects(ctx, query)
	if err != nil {
		return nil, err
	}
	items := make([]ProjectSummary, 0, len(rows))
	for i := range rows {
		count, err := s.repo.CountActiveEnvironments(ctx, rows[i].ID)
		if err != nil {
			return nil, err
		}
		items = append(items, projectSummary(&rows[i], count))
	}
	return &ListResponse{Items: items, Total: total, Page: page, PageSize: size}, nil
}

func (s *Service) GetProject(ctx context.Context, id uint64) (*ProjectDetail, error) {
	row, err := s.repo.GetProject(ctx, id)
	if err != nil {
		return nil, err
	}
	environments, err := s.environmentDetails(ctx, id)
	if err != nil {
		return nil, err
	}
	return &ProjectDetail{
		ProjectSummary: projectSummary(row, int64(len(environments))),
		Environments:   environments,
	}, nil
}

func (s *Service) ListEnvironments(ctx context.Context, projectID uint64) ([]Environment, error) {
	if _, err := s.repo.GetProject(ctx, projectID); err != nil {
		return nil, err
	}
	return s.environmentDetails(ctx, projectID)
}

func (s *Service) GetEnvironment(ctx context.Context, projectID, environmentID uint64) (*Environment, error) {
	row, err := s.repo.GetEnvironment(ctx, projectID, environmentID)
	if err != nil {
		return nil, err
	}
	return s.environmentDetail(ctx, row)
}

func (s *Service) CreateProject(ctx context.Context, actor uint64, ip, ua string, req CreateProjectRequest) (*ProjectDetail, error) {
	name, err := normalizeProjectName(req.Name)
	if err != nil {
		return nil, err
	}
	if err := validateDescription(req.Description); err != nil {
		return nil, err
	}
	if req.OwnerUserID != nil && *req.OwnerUserID == 0 {
		return nil, validationError("owner_user_id must be greater than zero")
	}
	row := &models.DeliveryProject{Name: name, Description: req.Description, OwnerUserID: cloneUint64(req.OwnerUserID)}
	if err := s.repo.CreateProject(ctx, row); err != nil {
		return nil, err
	}
	s.record(ctx, actor, "delivery.project.create", "delivery.project", row.ID, ip, ua, map[string]any{"name": row.Name})
	return &ProjectDetail{ProjectSummary: projectSummary(row, 0), Environments: []Environment{}}, nil
}

func (s *Service) UpdateProject(ctx context.Context, actor uint64, ip, ua string, id uint64, req UpdateProjectRequest) (*ProjectDetail, error) {
	row, err := s.repo.GetProject(ctx, id)
	if err != nil {
		return nil, err
	}
	fields := make(map[string]any)
	changed := make([]string, 0, 3)
	if req.Name != nil {
		name, err := normalizeProjectName(*req.Name)
		if err != nil {
			return nil, err
		}
		if name != row.Name {
			fields["name"] = name
			changed = append(changed, "name")
		}
	}
	if req.Description != nil {
		if err := validateDescription(*req.Description); err != nil {
			return nil, err
		}
		if *req.Description != row.Description {
			fields["description"] = *req.Description
			changed = append(changed, "description")
		}
	}
	if req.OwnerUserID != nil {
		if *req.OwnerUserID == 0 {
			if row.OwnerUserID != nil {
				fields["owner_user_id"] = nil
				changed = append(changed, "owner_user_id")
			}
		} else if row.OwnerUserID == nil || *row.OwnerUserID != *req.OwnerUserID {
			fields["owner_user_id"] = *req.OwnerUserID
			changed = append(changed, "owner_user_id")
		}
	}
	if len(fields) > 0 {
		if err := s.repo.UpdateProject(ctx, id, fields); err != nil {
			return nil, err
		}
		sort.Strings(changed)
		updatedName := row.Name
		if value, ok := fields["name"].(string); ok {
			updatedName = value
		}
		s.record(ctx, actor, "delivery.project.update", "delivery.project", id, ip, ua, map[string]any{
			"changed_fields": changed,
			"name":           updatedName,
		})
	}
	return s.GetProject(ctx, id)
}

func (s *Service) DeleteProject(ctx context.Context, actor uint64, ip, ua string, id uint64) error {
	var row *models.DeliveryProject
	if err := s.repo.Transaction(ctx, func(repo projectRepository) error {
		var err error
		row, err = repo.LockProject(ctx, id)
		if err != nil {
			return err
		}
		if err := s.ensureInactive(ctx, id); err != nil {
			return err
		}
		count, err := repo.CountActiveEnvironments(ctx, id)
		if err != nil {
			return err
		}
		if count > 0 {
			return environmentInUseError()
		}
		return repo.DeleteProject(ctx, id)
	}); err != nil {
		return err
	}
	s.record(ctx, actor, "delivery.project.delete", "delivery.project", id, ip, ua, map[string]any{"name": row.Name})
	return nil
}

func (s *Service) BindEnvironment(ctx context.Context, actor uint64, ip, ua string, projectID uint64, req BindEnvironmentRequest) (*Environment, error) {
	key, err := normalizeEnvironmentKey(req.EnvironmentKey)
	if err != nil {
		return nil, err
	}
	if err := validateDisplayName(req.DisplayName); err != nil {
		return nil, err
	}
	if req.ApplicationID == 0 {
		return nil, validationError("application_id is required")
	}
	var application *Application
	row := &models.DeliveryEnvironment{
		ProjectID: projectID, EnvironmentKey: key, DisplayName: req.DisplayName, ApplicationID: req.ApplicationID,
	}
	if err := s.repo.Transaction(ctx, func(repo projectRepository) error {
		if _, err := repo.LockProject(ctx, projectID); err != nil {
			return err
		}
		if err := repo.LockApplication(ctx, req.ApplicationID); err != nil {
			return err
		}
		var err error
		application, err = s.availableApplication(ctx, req.ApplicationID)
		if err != nil {
			return err
		}
		bound, err := repo.ApplicationBound(ctx, req.ApplicationID)
		if err != nil {
			return err
		}
		if bound {
			return applicationAlreadyBoundError()
		}
		existing, err := repo.ListEnvironments(ctx, projectID)
		if err != nil {
			return err
		}
		for i := range existing {
			// The schema intentionally has no project chart snapshot. If an
			// existing binding cannot be resolved, readApplication fails closed;
			// accepting a new chart identity would otherwise be guesswork.
			boundApplication, err := s.readApplication(ctx, existing[i].ApplicationID)
			if err != nil {
				return err
			}
			if boundApplication.ChartName != application.ChartName {
				return apperr.New(errs.CodeChartIdentityMismatch, errs.KeyChartIdentityMismatch, "all project environments must use the same chart name")
			}
		}
		return repo.CreateEnvironment(ctx, row)
	}); err != nil {
		return nil, err
	}
	out := environmentFrom(row, application)
	s.record(ctx, actor, "delivery.environment.bind", "delivery.environment", row.ID, ip, ua, environmentAuditPayload(row, application.ChartName))
	return &out, nil
}

func (s *Service) UpdateEnvironment(ctx context.Context, actor uint64, ip, ua string, projectID, environmentID uint64, req UpdateEnvironmentRequest) (*Environment, error) {
	row, err := s.repo.GetEnvironment(ctx, projectID, environmentID)
	if err != nil {
		return nil, err
	}
	if req.ApplicationID != nil && *req.ApplicationID != row.ApplicationID {
		return nil, apperr.New(errs.CodeApplicationAlreadyBound, errs.KeyApplicationAlreadyBound, "application binding is immutable")
	}
	fields := make(map[string]any)
	changed := make([]string, 0, 2)
	if req.EnvironmentKey != nil {
		key, err := normalizeEnvironmentKey(*req.EnvironmentKey)
		if err != nil {
			return nil, err
		}
		if key != row.EnvironmentKey {
			fields["environment_key"] = key
			changed = append(changed, "environment_key")
		}
	}
	if req.DisplayName != nil {
		if err := validateDisplayName(*req.DisplayName); err != nil {
			return nil, err
		}
		if *req.DisplayName != row.DisplayName {
			fields["display_name"] = *req.DisplayName
			changed = append(changed, "display_name")
		}
	}
	if len(fields) > 0 {
		if err := s.repo.UpdateEnvironment(ctx, projectID, environmentID, fields); err != nil {
			return nil, err
		}
		sort.Strings(changed)
		updatedKey := row.EnvironmentKey
		if value, ok := fields["environment_key"].(string); ok {
			updatedKey = value
		}
		s.record(ctx, actor, "delivery.environment.update", "delivery.environment", environmentID, ip, ua, map[string]any{
			"changed_fields":  changed,
			"environment_key": updatedKey,
			"project_id":      projectID,
		})
	}
	return s.GetEnvironment(ctx, projectID, environmentID)
}

func (s *Service) UnbindEnvironment(ctx context.Context, actor uint64, ip, ua string, projectID, environmentID uint64) error {
	if _, err := s.repo.GetEnvironment(ctx, projectID, environmentID); err != nil {
		return err
	}
	var row *models.DeliveryEnvironment
	var application *Application
	if err := s.repo.Transaction(ctx, func(repo projectRepository) error {
		if _, err := repo.LockProject(ctx, projectID); err != nil {
			return err
		}
		if err := s.ensureInactive(ctx, projectID); err != nil {
			return err
		}
		var err error
		row, err = repo.GetEnvironment(ctx, projectID, environmentID)
		if err != nil {
			return err
		}
		count, err := repo.CountPipelineReferences(ctx, environmentID)
		if err != nil {
			return err
		}
		if count > 0 {
			return environmentInUseError()
		}
		application, err = s.readApplication(ctx, row.ApplicationID)
		if err != nil {
			return err
		}
		return repo.DeleteEnvironment(ctx, projectID, environmentID)
	}); err != nil {
		return err
	}
	s.record(ctx, actor, "delivery.environment.unbind", "delivery.environment", environmentID, ip, ua, environmentAuditPayload(row, application.ChartName))
	return nil
}

func (s *Service) environmentDetails(ctx context.Context, projectID uint64) ([]Environment, error) {
	rows, err := s.repo.ListEnvironments(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]Environment, 0, len(rows))
	for i := range rows {
		detail, err := s.environmentDetail(ctx, &rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *detail)
	}
	return out, nil
}

func (s *Service) environmentDetail(ctx context.Context, row *models.DeliveryEnvironment) (*Environment, error) {
	application, err := s.readApplication(ctx, row.ApplicationID)
	if err != nil {
		return nil, err
	}
	out := environmentFrom(row, application)
	return &out, nil
}

func (s *Service) availableApplication(ctx context.Context, id uint64) (*Application, error) {
	application, err := s.readApplication(ctx, id)
	if err != nil {
		return nil, err
	}
	if !application.Installed {
		return nil, apperr.New(errs.CodeApplicationUnavailable, errs.KeyApplicationUnavailable, "application release is not installed")
	}
	return application, nil
}

func (s *Service) readApplication(ctx context.Context, id uint64) (*Application, error) {
	if s.apps == nil {
		return nil, apperr.New(errs.CodeExecutionUnavailable, errs.KeyExecutionUnavailable, "application reader is not configured")
	}
	application, err := s.apps.GetApplication(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && application == nil) {
		return nil, apperr.New(errs.CodeApplicationUnavailable, errs.KeyApplicationUnavailable, "application not found")
	}
	if err != nil {
		return nil, err
	}
	if application.ID != id || application.ChartRepoID == 0 || strings.TrimSpace(application.ChartName) == "" ||
		application.ClusterID == 0 || strings.TrimSpace(application.Namespace) == "" || strings.TrimSpace(application.ReleaseName) == "" {
		return nil, apperr.New(errs.CodeApplicationUnavailable, errs.KeyApplicationUnavailable, "application projection is incomplete")
	}
	return application, nil
}

func (s *Service) ensureInactive(ctx context.Context, projectID uint64) error {
	if s.activity == nil {
		return apperr.New(errs.CodeExecutionUnavailable, errs.KeyExecutionUnavailable, "project activity guard is not configured")
	}
	activity, err := s.activity.ProjectActivity(ctx, projectID)
	if err != nil {
		return err
	}
	if activity.OutcomeUnknown {
		return apperr.New(errs.CodeOutcomeUnknown, errs.KeyOutcomeUnknown, "project has a run with an unknown outcome")
	}
	if activity.Active {
		return apperr.New(errs.CodeActiveRun, errs.KeyActiveRun, "project has an active run")
	}
	return nil
}

func (s *Service) record(ctx context.Context, actor uint64, action, targetType string, id uint64, ip, ua string, payload map[string]any) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, audit.Event{
		UserID: actorPointer(actor), Action: action, TargetType: targetType,
		TargetID: strconv.FormatUint(id, 10), Payload: payload, IP: ip, UserAgent: ua,
	})
}

func normalizeProjectName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if count := utf8.RuneCountInString(name); count < 1 || count > 128 {
		return "", validationError("project name must contain between 1 and 128 characters")
	}
	return name, nil
}

func validateDescription(description string) error {
	if utf8.RuneCountInString(description) > 4096 {
		return validationError("project description must not exceed 4096 characters")
	}
	return nil
}

func validateDisplayName(name string) error {
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 128 {
		return validationError("environment display name must contain between 1 and 128 characters")
	}
	return nil
}

func normalizeEnvironmentKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	var normalized strings.Builder
	normalized.Grow(len(value))
	pendingSeparator := false
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			if pendingSeparator && normalized.Len() > 0 {
				normalized.WriteByte('-')
			}
			pendingSeparator = false
			normalized.WriteRune(char)
		case char >= 'A' && char <= 'Z':
			if pendingSeparator && normalized.Len() > 0 {
				normalized.WriteByte('-')
			}
			pendingSeparator = false
			normalized.WriteRune(unicode.ToLower(char))
		case char == '-' || char == '_' || unicode.IsSpace(char):
			pendingSeparator = normalized.Len() > 0
		default:
			return "", validationError("environment key contains unsupported characters")
		}
	}
	key := normalized.String()
	if len(key) < 1 || len(key) > 63 {
		return "", validationError("environment key must normalize to a DNS label between 1 and 63 characters")
	}
	return key, nil
}

func projectSummary(row *models.DeliveryProject, environmentCount int64) ProjectSummary {
	return ProjectSummary{
		ID: row.ID, Name: row.Name, Description: row.Description, OwnerUserID: cloneUint64(row.OwnerUserID),
		EnvironmentCount: environmentCount, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func environmentFrom(row *models.DeliveryEnvironment, application *Application) Environment {
	return Environment{
		ID: row.ID, ProjectID: row.ProjectID, EnvironmentKey: row.EnvironmentKey, DisplayName: row.DisplayName,
		ApplicationID: row.ApplicationID, ApplicationName: application.Name, ChartRepoID: application.ChartRepoID,
		ChartName: application.ChartName, Installed: application.Installed, ClusterID: application.ClusterID,
		Namespace: application.Namespace, ReleaseName: application.ReleaseName,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func environmentAuditPayload(row *models.DeliveryEnvironment, chartName string) map[string]any {
	return map[string]any{
		"application_id":  row.ApplicationID,
		"chart_name":      chartName,
		"environment_key": row.EnvironmentKey,
		"project_id":      row.ProjectID,
	}
}

func actorPointer(actor uint64) *uint64 {
	if actor == 0 {
		return nil
	}
	return &actor
}

func cloneUint64(value *uint64) *uint64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
