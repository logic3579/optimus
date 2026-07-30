package vpc

import (
	"context"
	"errors"
	"math"
	"testing"

	"gorm.io/gorm"

	apperr "optimus-be/internal/infra/errors"
	"optimus-be/internal/models"
)

var errSecretDB = errors.New("postgres password=do-not-leak")

type stubRepo struct {
	vpcs        []Summary
	vpcTotal    int64
	vpc         *models.AWSVPC
	subnets     []SubnetSummary
	subnetTotal int64
	listErr     error
	findErr     error
	subnetErr   error
	listFilter  ListFilter
	subFilter   SubnetListFilter
}

func (r *stubRepo) List(_ context.Context, filter ListFilter) ([]Summary, int64, error) {
	r.listFilter = filter
	return r.vpcs, r.vpcTotal, r.listErr
}

func (r *stubRepo) FindByID(_ context.Context, _ uint64) (*models.AWSVPC, error) {
	return r.vpc, r.findErr
}

func (r *stubRepo) ListSubnets(_ context.Context, filter SubnetListFilter) ([]SubnetSummary, int64, error) {
	r.subFilter = filter
	return r.subnets, r.subnetTotal, r.subnetErr
}

func TestServiceListDefaultsAndReturnsNonNilItems(t *testing.T) {
	repo := &stubRepo{}
	result, err := NewService(repo).List(context.Background(), ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Items == nil || len(result.Items) != 0 {
		t.Fatalf("items = %#v", result.Items)
	}
	if repo.listFilter.Page != 1 || repo.listFilter.Size != 20 {
		t.Fatalf("filter = %#v", repo.listFilter)
	}
}

func TestServiceListValidation(t *testing.T) {
	tests := []ListQuery{
		{AccountID: uint64(math.MaxInt64) + 1},
		{Page: -1, Size: 20},
		{Page: 1, Size: -1},
		{Page: 1, Size: 201},
		{Page: math.MaxInt, Size: 2},
	}
	for _, query := range tests {
		_, err := NewService(&stubRepo{}).List(context.Background(), query)
		assertBizCode(t, err, apperr.CodeValidation)
	}
}

func TestServiceDatabaseErrorsAreSafe(t *testing.T) {
	_, err := NewService(&stubRepo{listErr: errSecretDB}).List(context.Background(), ListQuery{})
	assertSafeDBError(t, err)

	vpc := &models.AWSVPC{ID: 7, CloudAccountID: 2, Region: "us-east-1", VPCID: "vpc-1"}
	_, err = NewService(&stubRepo{vpc: vpc, subnetErr: errSecretDB}).ListSubnetsByVPCRowID(
		context.Background(), 7, SubnetListQuery{},
	)
	assertSafeDBError(t, err)
}

func TestServiceListSubnetsScopesTupleAndDefaults(t *testing.T) {
	vpc := &models.AWSVPC{ID: 7, CloudAccountID: 2, Region: "us-east-1", VPCID: "vpc-1"}
	repo := &stubRepo{vpc: vpc}
	result, err := NewService(repo).ListSubnetsByVPCRowID(context.Background(), 7, SubnetListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Items == nil {
		t.Fatal("items must be an empty JSON array")
	}
	want := SubnetListFilter{CloudAccountID: 2, Region: "us-east-1", VPCID: "vpc-1", Page: 1, Size: 20}
	if repo.subFilter != want {
		t.Fatalf("filter = %#v, want %#v", repo.subFilter, want)
	}
}

func TestServiceListSubnetsNotFoundAndInvalidID(t *testing.T) {
	for _, id := range []uint64{0, uint64(math.MaxInt64) + 1} {
		_, err := NewService(&stubRepo{}).ListSubnetsByVPCRowID(context.Background(), id, SubnetListQuery{})
		assertBizCode(t, err, apperr.CodeValidation)
	}

	_, err := NewService(&stubRepo{findErr: gorm.ErrRecordNotFound}).ListSubnetsByVPCRowID(
		context.Background(), 1, SubnetListQuery{},
	)
	assertBizCode(t, err, apperr.CodeAssetsVPCNotFound)

	_, err = NewService(&stubRepo{findErr: errSecretDB}).ListSubnetsByVPCRowID(
		context.Background(), 1, SubnetListQuery{},
	)
	assertSafeDBError(t, err)
}

func TestServiceSubnetListValidation(t *testing.T) {
	vpc := &models.AWSVPC{ID: 1, CloudAccountID: 2, Region: "us-east-1", VPCID: "vpc-1"}
	for _, query := range []SubnetListQuery{
		{Page: -1, Size: 20},
		{Page: 1, Size: -1},
		{Page: 1, Size: 201},
		{Page: math.MaxInt, Size: 2},
	} {
		_, err := NewService(&stubRepo{vpc: vpc}).ListSubnetsByVPCRowID(context.Background(), 1, query)
		assertBizCode(t, err, apperr.CodeValidation)
	}
}

func assertBizCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	var biz *apperr.BizError
	if !errors.As(err, &biz) || biz.Code != code {
		t.Fatalf("error = %#v, want BizError code %d", err, code)
	}
}

func assertSafeDBError(t *testing.T, err error) {
	t.Helper()
	var biz *apperr.BizError
	if !errors.As(err, &biz) || biz.Code != apperr.CodeDBError {
		t.Fatalf("error = %#v", err)
	}
	if biz.Message != "database operation failed" || biz.MessageKey != "common.database_error" {
		t.Fatalf("unsafe BizError = %#v", biz)
	}
}
