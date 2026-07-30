package database

import (
	"context"
	"math"

	apperr "optimus-be/internal/infra/errors"
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
		query.Page = 1
	}
	if query.Size == 0 {
		query.Size = 20
	}
	if query.AccountID > math.MaxInt64 {
		return ListFilter{}, validationError("account_id exceeds database range")
	}
	if query.Page < 1 {
		return ListFilter{}, validationError("page must be at least 1")
	}
	if query.Size < 1 || query.Size > 200 {
		return ListFilter{}, validationError("size must be between 1 and 200")
	}
	if query.Page-1 > math.MaxInt/query.Size {
		return ListFilter{}, validationError("pagination offset exceeds supported range")
	}
	return ListFilter(query), nil
}

func validationError(message string) error {
	return apperr.New(apperr.CodeValidation, "common.validation", message)
}
