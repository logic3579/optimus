package instance

import (
	"context"
	"math"
	"strings"

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
	if query.Page < 1 {
		return ListFilter{}, validationError("page must be at least 1")
	}
	if query.Size < 1 || query.Size > maxSize {
		return ListFilter{}, validationError("size must be between 1 and 200")
	}
	if query.Page-1 > math.MaxInt/query.Size {
		return ListFilter{}, validationError("pagination offset exceeds supported range")
	}

	return ListFilter{
		AccountID: query.AccountID, Region: strings.TrimSpace(query.Region),
		State: strings.TrimSpace(query.State), VPCID: strings.TrimSpace(query.VPCID),
		Q: strings.TrimSpace(query.Q), IncludeDeleted: query.IncludeDeleted,
		Page: query.Page, Size: query.Size, Offset: (query.Page - 1) * query.Size,
	}, nil
}

func validationError(message string) error {
	return apperr.New(apperr.CodeValidation, "common.validation", message)
}
