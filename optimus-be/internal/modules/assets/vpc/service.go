package vpc

import (
	"context"
	"errors"
	"math"

	"gorm.io/gorm"
	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
	"optimus-be/internal/modules/assets/errs"
)

const (
	defaultPage = 1
	defaultSize = 20
	maxSize     = 200
)

type repository interface {
	List(context.Context, ListFilter) ([]Summary, int64, error)
	FindByID(context.Context, uint64) (*models.AWSVPC, error)
	ListSubnets(context.Context, SubnetListFilter) ([]SubnetSummary, int64, error)
}

type Service struct{ repo repository }

func NewService(repo repository) *Service { return &Service{repo: repo} }

func (s *Service) List(ctx context.Context, query ListQuery) (*ListResponse, error) {
	page, size, err := validatePagination(query.Page, query.Size)
	if err != nil {
		return nil, err
	}
	if query.AccountID > math.MaxInt64 {
		return nil, validationError("account_id exceeds database range")
	}
	items, total, err := s.repo.List(ctx, ListFilter{
		AccountID: query.AccountID, Region: query.Region, Q: query.Q,
		IncludeDeleted: query.IncludeDeleted, Page: page, Size: size,
	})
	if err != nil {
		return nil, databaseError(err)
	}
	if items == nil {
		items = []Summary{}
	}
	return &ListResponse{Items: items, Total: total}, nil
}

func (s *Service) ListSubnetsByVPCRowID(ctx context.Context, vpcRowID uint64, query SubnetListQuery) (*SubnetListResponse, error) {
	if vpcRowID == 0 || vpcRowID > math.MaxInt64 {
		return nil, validationError("vpc id is outside the supported range")
	}
	page, size, err := validatePagination(query.Page, query.Size)
	if err != nil {
		return nil, err
	}
	vpc, err := s.repo.FindByID(ctx, vpcRowID)
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && vpc == nil) {
		return nil, apperr.Wrap(err, errs.CodeAssetsVPCNotFound, errs.KeyVPCNotFound, "VPC not found")
	}
	if err != nil {
		return nil, databaseError(err)
	}
	items, total, err := s.repo.ListSubnets(ctx, SubnetListFilter{
		CloudAccountID: vpc.CloudAccountID, Region: vpc.Region, VPCID: vpc.VPCID,
		Q: query.Q, IncludeDeleted: query.IncludeDeleted, Page: page, Size: size,
	})
	if err != nil {
		return nil, databaseError(err)
	}
	if items == nil {
		items = []SubnetSummary{}
	}
	return &SubnetListResponse{Items: items, Total: total}, nil
}

func validatePagination(page, size int) (int, int, error) {
	if page == 0 {
		page = defaultPage
	}
	if size == 0 {
		size = defaultSize
	}
	if page < 1 {
		return 0, 0, validationError("page must be at least 1")
	}
	if size < 1 || size > maxSize {
		return 0, 0, validationError("size must be between 1 and 200")
	}
	if page-1 > math.MaxInt/size {
		return 0, 0, validationError("pagination offset exceeds supported range")
	}
	return page, size, nil
}

func validationError(message string) error {
	return apperr.New(apperr.CodeValidation, "common.validation", message)
}

func databaseError(err error) error {
	return apperr.Wrap(err, apperr.CodeDBError, "common.database_error", "database operation failed")
}
