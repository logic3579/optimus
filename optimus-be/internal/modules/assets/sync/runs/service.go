package runs

import (
	"context"
	"math"
	"time"

	apperr "optimus-be/internal/infra/errors"
)

const (
	defaultPage = 1
	defaultSize = 20
	maxSize     = 200
)

type listRepository interface {
	List(context.Context, ListFilter) ([]Summary, int64, error)
}

type Service struct{ repo listRepository }

func NewService(repo listRepository) *Service { return &Service{repo: repo} }

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, error) {
	filter, err := validateListQuery(query)
	if err != nil {
		return nil, err
	}
	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeDBError, "common.database_error", "database operation failed")
	}
	if items == nil {
		items = []Summary{}
	}
	return &ListResponse{Items: items, Total: total}, nil
}

func validateListQuery(query ListQuery) (ListFilter, error) {
	if query.Page == 0 {
		query.Page = defaultPage
	}
	if query.Size == 0 {
		query.Size = defaultSize
	}
	if query.AccountID > math.MaxInt64 {
		return ListFilter{}, validationError("account_id exceeds database range")
	}
	if !oneOf(query.ResourceType, "", "instance", "network", "database") {
		return ListFilter{}, validationError("invalid resource_type")
	}
	if !oneOf(query.Status, "", "running", "success", "failed", "skipped") {
		return ListFilter{}, validationError("invalid status")
	}
	if query.Page < 1 {
		return ListFilter{}, validationError("page must be at least 1")
	}
	if query.Size < 1 || query.Size > maxSize {
		return ListFilter{}, validationError("size must be between 1 and 200")
	}
	if query.Page-1 > math.MaxInt/query.Size {
		return ListFilter{}, validationError("pagination offset exceeds supported range")
	}

	filter := ListFilter{
		AccountID: query.AccountID, ResourceType: query.ResourceType, Status: query.Status,
		Page: query.Page, Size: query.Size,
	}
	if query.StartedAfter != "" {
		startedAfter, err := time.Parse(time.RFC3339, query.StartedAfter)
		if err != nil {
			return ListFilter{}, validationError("started_after must be RFC3339")
		}
		filter.StartedAfter = &startedAfter
	}
	return filter, nil
}

func validationError(message string) error {
	return apperr.New(apperr.CodeValidation, "common.validation", message)
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
