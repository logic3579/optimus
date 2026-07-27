package project

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/delivery/errs"
)

type memoryRepo struct {
	projects      map[uint64]*models.DeliveryProject
	environments  map[uint64]*models.DeliveryEnvironment
	pipelineRefs  map[uint64]int64
	nextProjectID uint64
	nextEnvID     uint64
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		projects: make(map[uint64]*models.DeliveryProject), environments: make(map[uint64]*models.DeliveryEnvironment),
		pipelineRefs: make(map[uint64]int64), nextProjectID: 1, nextEnvID: 1,
	}
}

func (r *memoryRepo) Transaction(_ context.Context, fn func(projectRepository) error) error {
	return fn(r)
}
func (r *memoryRepo) LockProject(ctx context.Context, id uint64) (*models.DeliveryProject, error) {
	return r.GetProject(ctx, id)
}

func (r *memoryRepo) ListProjects(_ context.Context, q ListQuery) ([]models.DeliveryProject, int64, error) {
	rows := make([]models.DeliveryProject, 0, len(r.projects))
	for _, row := range r.projects {
		if !row.DeletedAt.Valid {
			rows = append(rows, *row)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID > rows[j].ID })
	return rows, int64(len(rows)), nil
}
func (r *memoryRepo) GetProject(_ context.Context, id uint64) (*models.DeliveryProject, error) {
	row, ok := r.projects[id]
	if !ok || row.DeletedAt.Valid {
		return nil, projectNotFoundError()
	}
	copy := *row
	return &copy, nil
}
func (r *memoryRepo) CreateProject(_ context.Context, row *models.DeliveryProject) error {
	for _, existing := range r.projects {
		if !existing.DeletedAt.Valid && existing.Name == row.Name {
			return projectNameConflictError()
		}
	}
	row.ID = r.nextProjectID
	r.nextProjectID++
	row.CreatedAt, row.UpdatedAt = time.Now(), time.Now()
	copy := *row
	r.projects[row.ID] = &copy
	return nil
}
func (r *memoryRepo) UpdateProject(_ context.Context, id uint64, fields map[string]any) error {
	row, ok := r.projects[id]
	if !ok || row.DeletedAt.Valid {
		return projectNotFoundError()
	}
	if name, ok := fields["name"].(string); ok {
		for otherID, other := range r.projects {
			if otherID != id && !other.DeletedAt.Valid && other.Name == name {
				return projectNameConflictError()
			}
		}
		row.Name = name
	}
	if value, ok := fields["description"].(string); ok {
		row.Description = value
	}
	if value, exists := fields["owner_user_id"]; exists {
		if value == nil {
			row.OwnerUserID = nil
		} else {
			v := value.(uint64)
			row.OwnerUserID = &v
		}
	}
	row.UpdatedAt = time.Now()
	return nil
}
func (r *memoryRepo) DeleteProject(_ context.Context, id uint64) error {
	row, ok := r.projects[id]
	if !ok || row.DeletedAt.Valid {
		return projectNotFoundError()
	}
	row.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}
	return nil
}
func (r *memoryRepo) ListEnvironments(_ context.Context, projectID uint64) ([]models.DeliveryEnvironment, error) {
	rows := make([]models.DeliveryEnvironment, 0)
	for _, row := range r.environments {
		if row.ProjectID == projectID && !row.DeletedAt.Valid {
			rows = append(rows, *row)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].EnvironmentKey == rows[j].EnvironmentKey {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].EnvironmentKey < rows[j].EnvironmentKey
	})
	return rows, nil
}
func (r *memoryRepo) GetEnvironment(_ context.Context, projectID, id uint64) (*models.DeliveryEnvironment, error) {
	row, ok := r.environments[id]
	if !ok || row.DeletedAt.Valid || row.ProjectID != projectID {
		return nil, environmentNotFoundError()
	}
	copy := *row
	return &copy, nil
}
func (r *memoryRepo) ApplicationBound(_ context.Context, applicationID uint64) (bool, error) {
	for _, row := range r.environments {
		if row.ApplicationID == applicationID && !row.DeletedAt.Valid {
			return true, nil
		}
	}
	return false, nil
}
func (r *memoryRepo) CreateEnvironment(_ context.Context, row *models.DeliveryEnvironment) error {
	for _, existing := range r.environments {
		if existing.DeletedAt.Valid {
			continue
		}
		if existing.ApplicationID == row.ApplicationID {
			return applicationAlreadyBoundError()
		}
		if existing.ProjectID == row.ProjectID && existing.EnvironmentKey == row.EnvironmentKey {
			return environmentInUseError()
		}
	}
	row.ID = r.nextEnvID
	r.nextEnvID++
	row.CreatedAt, row.UpdatedAt = time.Now(), time.Now()
	copy := *row
	r.environments[row.ID] = &copy
	return nil
}
func (r *memoryRepo) UpdateEnvironment(_ context.Context, projectID, id uint64, fields map[string]any) error {
	row, ok := r.environments[id]
	if !ok || row.DeletedAt.Valid || row.ProjectID != projectID {
		return environmentNotFoundError()
	}
	if key, ok := fields["environment_key"].(string); ok {
		for otherID, other := range r.environments {
			if otherID != id && !other.DeletedAt.Valid && other.ProjectID == projectID && other.EnvironmentKey == key {
				return environmentInUseError()
			}
		}
		row.EnvironmentKey = key
	}
	if display, ok := fields["display_name"].(string); ok {
		row.DisplayName = display
	}
	row.UpdatedAt = time.Now()
	return nil
}
func (r *memoryRepo) DeleteEnvironment(_ context.Context, projectID, id uint64) error {
	row, ok := r.environments[id]
	if !ok || row.DeletedAt.Valid || row.ProjectID != projectID {
		return environmentNotFoundError()
	}
	row.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}
	return nil
}
func (r *memoryRepo) CountActiveEnvironments(_ context.Context, projectID uint64) (int64, error) {
	rows, _ := r.ListEnvironments(context.Background(), projectID)
	return int64(len(rows)), nil
}
func (r *memoryRepo) CountPipelineReferences(_ context.Context, environmentID uint64) (int64, error) {
	return r.pipelineRefs[environmentID], nil
}

type applicationReaderStub struct {
	apps map[uint64]Application
	err  map[uint64]error
}

func (s *applicationReaderStub) GetApplication(_ context.Context, id uint64) (*Application, error) {
	if err := s.err[id]; err != nil {
		return nil, err
	}
	app, ok := s.apps[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copy := app
	return &copy, nil
}

type activityReaderStub struct{ activity ProjectActivity }

func (s activityReaderStub) ProjectActivity(context.Context, uint64) (ProjectActivity, error) {
	return s.activity, nil
}

type auditStub struct{ events []audit.Event }

func (s *auditStub) Record(_ context.Context, event audit.Event) error {
	s.events = append(s.events, event)
	return nil
}

func setupService() (*Service, *memoryRepo, *applicationReaderStub, *auditStub) {
	repo := newMemoryRepo()
	apps := &applicationReaderStub{apps: map[uint64]Application{
		10: {ID: 10, Name: "payments-dev", ChartRepoID: 2, ChartName: "payments", Installed: true, ClusterID: 3, Namespace: "dev", ReleaseName: "payments"},
		11: {ID: 11, Name: "payments-prod", ChartRepoID: 2, ChartName: "payments", Installed: true, ClusterID: 4, Namespace: "prod", ReleaseName: "payments"},
		12: {ID: 12, Name: "catalog-prod", ChartRepoID: 5, ChartName: "catalog", Installed: true, ClusterID: 4, Namespace: "prod", ReleaseName: "catalog"},
	}, err: make(map[uint64]error)}
	audits := &auditStub{}
	return NewService(repo, apps, activityReaderStub{}, audits), repo, apps, audits
}

func bizCode(t *testing.T, err error) apperr.Code {
	t.Helper()
	var biz *apperr.BizError
	require.ErrorAs(t, err, &biz)
	return biz.Code
}

func TestServiceProjectLifecycleAndAudit(t *testing.T) {
	svc, repo, _, audits := setupService()
	ctx := context.Background()
	created, err := svc.CreateProject(ctx, 0, "127.0.0.1", "test", CreateProjectRequest{Name: " payments ", Description: "delivery"})
	require.NoError(t, err)
	require.Equal(t, "payments", created.Name)
	require.Nil(t, created.OwnerUserID)
	name, description, owner := "payments-api", "new", uint64(42)
	updated, err := svc.UpdateProject(ctx, 7, "127.0.0.2", "ua", created.ID, UpdateProjectRequest{Name: &name, Description: &description, OwnerUserID: &owner})
	require.NoError(t, err)
	require.Equal(t, "payments-api", updated.Name)
	require.Equal(t, owner, *updated.OwnerUserID)
	require.NoError(t, svc.DeleteProject(ctx, 7, "ip", "ua", created.ID))
	require.True(t, repo.projects[created.ID].DeletedAt.Valid)
	require.Len(t, audits.events, 3)
	require.Equal(t, []string{"delivery.project.create", "delivery.project.update", "delivery.project.delete"}, []string{audits.events[0].Action, audits.events[1].Action, audits.events[2].Action})
	require.Nil(t, audits.events[0].UserID)
	require.Equal(t, map[string]any{"name": "payments"}, audits.events[0].Payload)
	require.Equal(t, "delivery.project", audits.events[1].TargetType)
	require.Equal(t, map[string]any{"changed_fields": []string{"description", "name", "owner_user_id"}, "name": "payments-api"}, audits.events[1].Payload)
	require.Equal(t, map[string]any{"name": "payments-api"}, audits.events[2].Payload)
}

func TestServiceProjectDuplicateAndSoftDeleteReleasesName(t *testing.T) {
	svc, _, _, _ := setupService()
	ctx := context.Background()
	one, err := svc.CreateProject(ctx, 0, "", "", CreateProjectRequest{Name: "same"})
	require.NoError(t, err)
	two, err := svc.CreateProject(ctx, 0, "", "", CreateProjectRequest{Name: "other"})
	require.NoError(t, err)
	_, err = svc.CreateProject(ctx, 0, "", "", CreateProjectRequest{Name: "same"})
	require.Equal(t, errs.CodeProjectNameConflict, bizCode(t, err))
	duplicate := one.Name
	_, err = svc.UpdateProject(ctx, 0, "", "", two.ID, UpdateProjectRequest{Name: &duplicate})
	require.Equal(t, errs.CodeProjectNameConflict, bizCode(t, err))
	require.NoError(t, svc.DeleteProject(ctx, 0, "", "", one.ID))
	_, err = svc.CreateProject(ctx, 0, "", "", CreateProjectRequest{Name: "same"})
	require.NoError(t, err)
}

func TestServiceBindingValidatesApplicationChartAndKey(t *testing.T) {
	svc, _, apps, _ := setupService()
	ctx := context.Background()
	project, err := svc.CreateProject(ctx, 0, "", "", CreateProjectRequest{Name: "payments"})
	require.NoError(t, err)
	env, err := svc.BindEnvironment(ctx, 5, "ip", "ua", project.ID, BindEnvironmentRequest{EnvironmentKey: " Dev_US ", DisplayName: "  Development  ", ApplicationID: 10})
	require.NoError(t, err)
	require.Equal(t, "dev-us", env.EnvironmentKey)
	require.Equal(t, "  Development  ", env.DisplayName, "display name must not be mutated")
	require.Equal(t, "payments", env.ChartName)

	_, err = svc.BindEnvironment(ctx, 0, "", "", project.ID, BindEnvironmentRequest{EnvironmentKey: "prod", DisplayName: "Prod", ApplicationID: 10})
	require.Equal(t, errs.CodeApplicationAlreadyBound, bizCode(t, err))
	_, err = svc.BindEnvironment(ctx, 0, "", "", project.ID, BindEnvironmentRequest{EnvironmentKey: "prod", DisplayName: "Prod", ApplicationID: 12})
	require.Equal(t, errs.CodeChartIdentityMismatch, bizCode(t, err))
	_, err = svc.BindEnvironment(ctx, 0, "", "", project.ID, BindEnvironmentRequest{EnvironmentKey: "!!!", DisplayName: "Prod", ApplicationID: 11})
	require.Equal(t, apperr.CodeValidation, bizCode(t, err))
	uninstalled := apps.apps[11]
	uninstalled.Installed = false
	apps.apps[11] = uninstalled
	_, err = svc.BindEnvironment(ctx, 0, "", "", project.ID, BindEnvironmentRequest{EnvironmentKey: "prod", DisplayName: "Prod", ApplicationID: 11})
	require.Equal(t, errs.CodeApplicationUnavailable, bizCode(t, err))
	delete(apps.apps, 11)
	_, err = svc.BindEnvironment(ctx, 0, "", "", project.ID, BindEnvironmentRequest{EnvironmentKey: "prod", DisplayName: "Prod", ApplicationID: 11})
	require.Equal(t, errs.CodeApplicationUnavailable, bizCode(t, err))
}

func TestServiceChartIdentityResolutionFailsClosedForMissingExistingApplication(t *testing.T) {
	svc, repo, apps, _ := setupService()
	ctx := context.Background()
	project, _ := svc.CreateProject(ctx, 0, "", "", CreateProjectRequest{Name: "p"})
	repo.environments[1] = &models.DeliveryEnvironment{ID: 1, ProjectID: project.ID, EnvironmentKey: "dev", DisplayName: "Dev", ApplicationID: 99}
	repo.nextEnvID = 2
	apps.err[99] = gorm.ErrRecordNotFound
	_, err := svc.BindEnvironment(ctx, 0, "", "", project.ID, BindEnvironmentRequest{EnvironmentKey: "prod", DisplayName: "Prod", ApplicationID: 11})
	require.Equal(t, errs.CodeApplicationUnavailable, bizCode(t, err))
}

func TestServiceEnvironmentUpdateImmutableBindingOrderingAndAudit(t *testing.T) {
	svc, _, _, audits := setupService()
	ctx := context.Background()
	project, _ := svc.CreateProject(ctx, 0, "", "", CreateProjectRequest{Name: "p"})
	prod, err := svc.BindEnvironment(ctx, 9, "ip", "ua", project.ID, BindEnvironmentRequest{EnvironmentKey: "prod", DisplayName: "Production", ApplicationID: 11})
	require.NoError(t, err)
	_, err = svc.BindEnvironment(ctx, 9, "ip", "ua", project.ID, BindEnvironmentRequest{EnvironmentKey: "dev", DisplayName: "Development", ApplicationID: 10})
	require.NoError(t, err)
	other := uint64(12)
	_, err = svc.UpdateEnvironment(ctx, 9, "ip", "ua", project.ID, prod.ID, UpdateEnvironmentRequest{ApplicationID: &other})
	require.Equal(t, errs.CodeApplicationAlreadyBound, bizCode(t, err))
	key, display := " QA_ENV ", " Quality Assurance "
	updated, err := svc.UpdateEnvironment(ctx, 9, "ip", "ua", project.ID, prod.ID, UpdateEnvironmentRequest{EnvironmentKey: &key, DisplayName: &display})
	require.NoError(t, err)
	require.Equal(t, "qa-env", updated.EnvironmentKey)
	require.Equal(t, display, updated.DisplayName)
	detail, err := svc.GetProject(ctx, project.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"dev", "qa-env"}, []string{detail.Environments[0].EnvironmentKey, detail.Environments[1].EnvironmentKey})
	last := audits.events[len(audits.events)-1]
	require.Equal(t, "delivery.environment.update", last.Action)
	require.Equal(t, map[string]any{"changed_fields": []string{"display_name", "environment_key"}, "environment_key": "qa-env", "project_id": project.ID}, last.Payload)
}

func TestServiceEnvironmentUnbindGuardsAndReleasesBinding(t *testing.T) {
	ctx := context.Background()
	makeBound := func(activity ProjectActivity) (*Service, *memoryRepo, uint64, uint64) {
		svc, repo, apps, audits := setupService()
		svc = NewService(repo, apps, activityReaderStub{activity: activity}, audits)
		project, _ := svc.CreateProject(ctx, 0, "", "", CreateProjectRequest{Name: "p"})
		env, _ := svc.BindEnvironment(ctx, 0, "", "", project.ID, BindEnvironmentRequest{EnvironmentKey: "dev", DisplayName: "Dev", ApplicationID: 10})
		return svc, repo, project.ID, env.ID
	}
	svc, _, projectID, environmentID := makeBound(ProjectActivity{Active: true})
	require.Equal(t, errs.CodeActiveRun, bizCode(t, svc.UnbindEnvironment(ctx, 0, "", "", projectID, environmentID)))
	svc, _, projectID, environmentID = makeBound(ProjectActivity{OutcomeUnknown: true})
	require.Equal(t, errs.CodeOutcomeUnknown, bizCode(t, svc.UnbindEnvironment(ctx, 0, "", "", projectID, environmentID)))
	svc, repo, projectID, environmentID := makeBound(ProjectActivity{})
	repo.pipelineRefs[environmentID] = 1
	require.Equal(t, errs.CodeEnvironmentInUse, bizCode(t, svc.UnbindEnvironment(ctx, 0, "", "", projectID, environmentID)))
	repo.pipelineRefs[environmentID] = 0
	require.NoError(t, svc.UnbindEnvironment(ctx, 0, "", "", projectID, environmentID))
	_, err := svc.BindEnvironment(ctx, 0, "", "", projectID, BindEnvironmentRequest{EnvironmentKey: "dev", DisplayName: "Dev again", ApplicationID: 10})
	require.NoError(t, err, "soft delete must release environment key and application binding")

	nilGuardService := NewService(repo, &applicationReaderStub{apps: map[uint64]Application{10: {
		ID: 10, Name: "payments-dev", ChartRepoID: 2, ChartName: "payments", Installed: true,
		ClusterID: 3, Namespace: "dev", ReleaseName: "payments",
	}}, err: map[uint64]error{}}, nil, &auditStub{})
	rebound, err := nilGuardService.GetProject(ctx, projectID)
	require.NoError(t, err)
	require.Equal(t, errs.CodeExecutionUnavailable, bizCode(t, nilGuardService.UnbindEnvironment(ctx, 0, "", "", projectID, rebound.Environments[0].ID)))
}

func TestServiceProjectDeleteGuardsBindingsAndNilGuard(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name     string
		activity ProjectActivity
		code     apperr.Code
	}{
		{name: "active", activity: ProjectActivity{Active: true}, code: errs.CodeActiveRun},
		{name: "outcome unknown", activity: ProjectActivity{OutcomeUnknown: true}, code: errs.CodeOutcomeUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, apps, audits := setupService()
			svc = NewService(repo, apps, activityReaderStub{activity: tc.activity}, audits)
			project, _ := svc.CreateProject(ctx, 0, "", "", CreateProjectRequest{Name: "p"})
			require.Equal(t, tc.code, bizCode(t, svc.DeleteProject(ctx, 0, "", "", project.ID)))
		})
	}
	svc, repo, _, _ := setupService()
	project, _ := svc.CreateProject(ctx, 0, "", "", CreateProjectRequest{Name: "bound"})
	_, _ = svc.BindEnvironment(ctx, 0, "", "", project.ID, BindEnvironmentRequest{EnvironmentKey: "dev", DisplayName: "Dev", ApplicationID: 10})
	require.Equal(t, errs.CodeEnvironmentInUse, bizCode(t, svc.DeleteProject(ctx, 0, "", "", project.ID)))
	nilGuard := NewService(repo, &applicationReaderStub{apps: map[uint64]Application{}, err: map[uint64]error{}}, nil, &auditStub{})
	empty, _ := nilGuard.CreateProject(ctx, 0, "", "", CreateProjectRequest{Name: "empty"})
	require.Equal(t, errs.CodeExecutionUnavailable, bizCode(t, nilGuard.DeleteProject(ctx, 0, "", "", empty.ID)))
}

func TestServiceListAndGetUseDeterministicEnvironmentOrder(t *testing.T) {
	svc, _, _, _ := setupService()
	ctx := context.Background()
	first, _ := svc.CreateProject(ctx, 0, "", "", CreateProjectRequest{Name: "first"})
	second, _ := svc.CreateProject(ctx, 0, "", "", CreateProjectRequest{Name: "second"})
	_, _ = svc.BindEnvironment(ctx, 0, "", "", first.ID, BindEnvironmentRequest{EnvironmentKey: "prod", DisplayName: "Prod", ApplicationID: 11})
	_, _ = svc.BindEnvironment(ctx, 0, "", "", first.ID, BindEnvironmentRequest{EnvironmentKey: "dev", DisplayName: "Dev", ApplicationID: 10})
	page, err := svc.ListProjects(ctx, ListQuery{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, []uint64{second.ID, first.ID}, []uint64{page.Items[0].ID, page.Items[1].ID})
	require.Equal(t, int64(2), page.Items[1].EnvironmentCount)
	detail, err := svc.GetProject(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"dev", "prod"}, []string{detail.Environments[0].EnvironmentKey, detail.Environments[1].EnvironmentKey})
}

func TestAuditPayloadsContainOnlyExplicitSafeFields(t *testing.T) {
	svc, _, _, audits := setupService()
	ctx := context.Background()
	project, _ := svc.CreateProject(ctx, 2, "ip", "ua", CreateProjectRequest{Name: "p"})
	env, _ := svc.BindEnvironment(ctx, 2, "ip", "ua", project.ID, BindEnvironmentRequest{EnvironmentKey: "dev", DisplayName: "Dev", ApplicationID: 10})
	require.NoError(t, svc.UnbindEnvironment(ctx, 2, "ip", "ua", project.ID, env.ID))
	bindEvent := audits.events[1]
	require.Equal(t, "delivery.environment.bind", bindEvent.Action)
	require.Equal(t, map[string]any{"application_id": uint64(10), "chart_name": "payments", "environment_key": "dev", "project_id": project.ID}, bindEvent.Payload)
	unbindEvent := audits.events[2]
	require.Equal(t, "delivery.environment.unbind", unbindEvent.Action)
	require.Equal(t, bindEvent.Payload, unbindEvent.Payload)
	for _, event := range audits.events {
		require.False(t, reflect.ValueOf(event.Payload).IsZero())
	}
}
