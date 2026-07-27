package pipeline

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin/binding"
	"github.com/stretchr/testify/require"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/audit"
	"optimus-be/internal/modules/delivery/errs"
)

type memoryRepository struct {
	mu           sync.Mutex
	projectID    uint64
	environments []models.DeliveryEnvironment
	current      *models.DeliveryPipeline
	stages       map[uint64][]models.DeliveryPipelineStage
	nextID       uint64
	projectLock  bool
	lockAcquired chan struct{}
	releaseLock  chan struct{}
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		projectID: 1,
		environments: []models.DeliveryEnvironment{
			{ID: 10, ProjectID: 1, EnvironmentKey: "development"},
			{ID: 30, ProjectID: 1, EnvironmentKey: "production"},
		},
		stages: make(map[uint64][]models.DeliveryPipelineStage),
		nextID: 1,
	}
}

func (r *memoryRepository) Transaction(ctx context.Context, fn func(repository) error) error {
	err := fn(r)
	if err != nil {
		return err
	}
	r.mu.Lock()
	if r.projectLock {
		r.projectLock = false
	}
	r.mu.Unlock()
	return nil
}

func (r *memoryRepository) LockProject(context.Context, uint64) error {
	r.mu.Lock()
	if r.projectLock {
		r.mu.Unlock()
		return pipelineVersionConflictError()
	}
	r.projectLock = true
	acquired, release := r.lockAcquired, r.releaseLock
	r.mu.Unlock()
	if acquired != nil {
		close(acquired)
	}
	if release != nil {
		<-release
	}
	return nil
}

func (r *memoryRepository) ListEnvironments(context.Context, uint64) ([]models.DeliveryEnvironment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]models.DeliveryEnvironment(nil), r.environments...), nil
}

func (r *memoryRepository) GetCurrent(context.Context, uint64) (*models.DeliveryPipeline, []models.DeliveryPipelineStage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil {
		return nil, nil, nil
	}
	row := *r.current
	return &row, append([]models.DeliveryPipelineStage(nil), r.stages[row.ID]...), nil
}

func (r *memoryRepository) ClearCurrent(context.Context, uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current != nil {
		r.current.IsCurrent = false
	}
	return nil
}

func (r *memoryRepository) CreatePipeline(_ context.Context, row *models.DeliveryPipeline, stages []models.DeliveryPipelineStage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	row.ID = r.nextID
	for i := range stages {
		stages[i].ID = uint64(100 + i + row.Version*10)
		stages[i].PipelineID = row.ID
	}
	copyRow := *row
	r.current = &copyRow
	r.stages[row.ID] = append([]models.DeliveryPipelineStage(nil), stages...)
	return nil
}

type auditRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *auditRecorder) Record(_ context.Context, event audit.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	return nil
}

func newServiceForTest() (*Service, *memoryRepository, *auditRecorder) {
	repo := newMemoryRepository()
	recorder := &auditRecorder{}
	return NewService(repo, recorder, 30*time.Minute), repo, recorder
}

func TestPublishRequestBindingRequiresOneToTwentyStages(t *testing.T) {
	require.Error(t, binding.Validator.ValidateStruct(PublishRequest{}))
	require.NoError(t, binding.Validator.ValidateStruct(PublishRequest{Stages: []StageInput{{EnvironmentID: 10}}}))
	twentyOne := make([]StageInput, 21)
	for i := range twentyOne {
		twentyOne[i].EnvironmentID = uint64(i + 1)
	}
	require.Error(t, binding.Validator.ValidateStruct(PublishRequest{Stages: twentyOne}))
	require.Error(t, binding.Validator.ValidateStruct(PublishRequest{Stages: []StageInput{{}}}))
}

func TestPublishRejectsInvalidStages(t *testing.T) {
	tests := []struct {
		name string
		req  PublishRequest
	}{
		{name: "empty stages", req: PublishRequest{}},
		{name: "duplicate environment", req: PublishRequest{Stages: []StageInput{{EnvironmentID: 10, Timeout: time.Minute}, {EnvironmentID: 10, Timeout: time.Minute}}}},
		{name: "missing binding", req: PublishRequest{Stages: []StageInput{{EnvironmentID: 99, Timeout: time.Minute}}}},
		{name: "zero timeout", req: PublishRequest{Stages: []StageInput{{EnvironmentID: 10}}}},
		{name: "negative timeout", req: PublishRequest{Stages: []StageInput{{EnvironmentID: 10, Timeout: -time.Second}}}},
		{name: "timeout above configured maximum", req: PublishRequest{Stages: []StageInput{{EnvironmentID: 10, Timeout: 30*time.Minute + time.Second}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, recorder := newServiceForTest()
			_, err := svc.Publish(context.Background(), 7, "127.0.0.1", "test", 1, tc.req)
			require.Equal(t, errs.CodePipelineInvalid, businessCode(t, err))
			require.Empty(t, recorder.events)
		})
	}
}

func TestPublishHonorsConfiguredTimeoutAboveThirtyMinutes(t *testing.T) {
	repo := newMemoryRepository()
	recorder := &auditRecorder{}
	svc := NewService(repo, recorder, 45*time.Minute)

	result, err := svc.Publish(context.Background(), 7, "", "", 1, PublishRequest{
		Stages: []StageInput{{EnvironmentID: 10, Timeout: 45 * time.Minute}},
	})
	require.NoError(t, err)
	require.Equal(t, 45*time.Minute, result.Stages[0].Timeout)

	_, err = svc.Publish(context.Background(), 7, "", "", 1, PublishRequest{
		Stages: []StageInput{{EnvironmentID: 10, Timeout: 45*time.Minute + time.Second}},
	})
	require.Equal(t, errs.CodePipelineInvalid, businessCode(t, err))
}

func TestPublishNormalizesOrderPersistsClosedExecutorAndAuditsSafeIDs(t *testing.T) {
	svc, repo, recorder := newServiceForTest()
	result, err := svc.Publish(context.Background(), 7, "127.0.0.1", "test", 1, PublishRequest{Stages: []StageInput{
		{EnvironmentID: 30, ApprovalRequired: true, Timeout: 20 * time.Minute},
		{EnvironmentID: 10, Timeout: 5 * time.Minute},
	}})
	require.NoError(t, err)
	require.Equal(t, 1, result.Version)
	require.Equal(t, []int{1, 2}, []int{result.Stages[0].Order, result.Stages[1].Order})
	require.Equal(t, []uint64{30, 10}, []uint64{result.Stages[0].EnvironmentID, result.Stages[1].EnvironmentID})
	for _, stage := range repo.stages[result.ID] {
		require.Equal(t, models.DeliveryExecutorHelmUpgradeExistingRelease, stage.Executor)
	}
	require.Len(t, recorder.events, 1)
	event := recorder.events[0]
	require.Equal(t, "delivery.pipeline.publish", event.Action)
	require.Equal(t, "delivery.pipeline", event.TargetType)
	require.Equal(t, map[string]any{
		"version":         1,
		"stage_ids":       []uint64{result.Stages[0].ID, result.Stages[1].ID},
		"environment_ids": []uint64{30, 10},
	}, event.Payload)
}

func TestPublishLeavesOldVersionStagesImmutable(t *testing.T) {
	svc, repo, _ := newServiceForTest()
	old := &models.DeliveryPipeline{ID: 1, ProjectID: 1, Version: 1, IsCurrent: true}
	repo.current = old
	repo.stages[old.ID] = []models.DeliveryPipelineStage{{ID: 11, PipelineID: 1, EnvironmentID: 10, StageOrder: 1, Executor: models.DeliveryExecutorHelmUpgradeExistingRelease, TimeoutSeconds: 300}}
	wantOldStages := append([]models.DeliveryPipelineStage(nil), repo.stages[old.ID]...)

	result, err := svc.Publish(context.Background(), 7, "", "", 1, PublishRequest{Stages: []StageInput{{EnvironmentID: 30, Timeout: 10 * time.Minute}}})
	require.NoError(t, err)
	require.Equal(t, 2, result.Version)
	require.Equal(t, wantOldStages, repo.stages[old.ID])
	require.False(t, old.IsCurrent)
}

func TestConcurrentPublicationCreatesVersionTwoAndOneRetryableConflict(t *testing.T) {
	svc, repo, _ := newServiceForTest()
	repo.current = &models.DeliveryPipeline{ID: 1, ProjectID: 1, Version: 1, IsCurrent: true}
	repo.stages[1] = []models.DeliveryPipelineStage{{ID: 11, PipelineID: 1, EnvironmentID: 10, StageOrder: 1, Executor: models.DeliveryExecutorHelmUpgradeExistingRelease, TimeoutSeconds: 300}}
	repo.lockAcquired = make(chan struct{})
	repo.releaseLock = make(chan struct{})

	type publication struct {
		pipeline *Pipeline
		err      error
	}
	results := make(chan publication, 2)
	request := PublishRequest{Stages: []StageInput{{EnvironmentID: 30, Timeout: time.Minute}}}
	go func() {
		pipeline, err := svc.Publish(context.Background(), 7, "", "", 1, request)
		results <- publication{pipeline: pipeline, err: err}
	}()
	<-repo.lockAcquired
	go func() {
		pipeline, err := svc.Publish(context.Background(), 8, "", "", 1, request)
		results <- publication{pipeline: pipeline, err: err}
	}()
	conflict := <-results
	require.Nil(t, conflict.pipeline)
	require.Equal(t, errs.CodePipelineVersionConflict, businessCode(t, conflict.err))
	close(repo.releaseLock)
	success := <-results
	require.NoError(t, success.err)
	require.Equal(t, 2, success.pipeline.Version)
}

func TestPipelinePublicShapeDoesNotExposeArbitraryExecutorInput(t *testing.T) {
	typeOfStage := reflect.TypeOf(StageInput{})
	_, exists := typeOfStage.FieldByName("Executor")
	require.False(t, exists)
}

func businessCode(t *testing.T, err error) apperr.Code {
	t.Helper()
	var business *apperr.BizError
	require.ErrorAs(t, err, &business)
	return business.Code
}
