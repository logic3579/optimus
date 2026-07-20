package instance

import (
	"context"
	"errors"
	"math"
	"testing"

	apperr "optimus-be/internal/infra/errors"
)

type stubListRepo struct {
	items  []Summary
	total  int64
	err    error
	filter ListFilter
}

func (r *stubListRepo) List(_ context.Context, filter ListFilter) ([]Summary, int64, error) {
	r.filter = filter
	return r.items, r.total, r.err
}

func TestServiceListDefaultsAndReturnsEmptyArray(t *testing.T) {
	repo := &stubListRepo{}
	result, err := NewService(repo).List(context.Background(), ListQuery{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if repo.filter.Page != 1 || repo.filter.Size != 20 || repo.filter.Offset != 0 {
		t.Fatalf("filter pagination = %#v", repo.filter)
	}
	if result.Items == nil {
		t.Fatal("Items must encode as an empty array, not null")
	}
}

func TestServiceListValidatesDatabaseRangeAndPagination(t *testing.T) {
	tests := []struct {
		name  string
		query ListQuery
	}{
		{name: "account beyond bigint", query: ListQuery{AccountID: uint64(math.MaxInt64) + 1}},
		{name: "negative page", query: ListQuery{Page: -1}},
		{name: "negative size", query: ListQuery{Size: -1}},
		{name: "oversize", query: ListQuery{Size: 201}},
		{name: "offset overflow", query: ListQuery{Page: math.MaxInt, Size: 200}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewService(&stubListRepo{}).List(context.Background(), test.query)
			var biz *apperr.BizError
			if !errors.As(err, &biz) || biz.Code != apperr.CodeValidation {
				t.Fatalf("List() error = %#v, want validation BizError", err)
			}
		})
	}
}

func TestServiceListPassesFiltersAndPagination(t *testing.T) {
	repo := &stubListRepo{items: []Summary{{ID: 1}}, total: 1}
	result, err := NewService(repo).List(context.Background(), ListQuery{
		AccountID: 7, Region: "us-east-1", State: "running", VPCID: "vpc-a",
		Q: "payments", IncludeDeleted: true, Page: 3, Size: 25,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := ListFilter{
		AccountID: 7, Region: "us-east-1", State: "running", VPCID: "vpc-a",
		Q: "payments", IncludeDeleted: true, Page: 3, Size: 25, Offset: 50,
	}
	if repo.filter != want {
		t.Fatalf("filter = %#v, want %#v", repo.filter, want)
	}
	if result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestServiceListMapsRepositoryErrorSafely(t *testing.T) {
	secret := errors.New("postgres password=do-not-leak")
	_, err := NewService(&stubListRepo{err: secret}).List(context.Background(), ListQuery{})
	var biz *apperr.BizError
	if !errors.As(err, &biz) || biz.Code != apperr.CodeDBError {
		t.Fatalf("List() error = %#v, want database BizError", err)
	}
	if biz.Message != "database operation failed" || biz.MessageKey != "common.database_error" {
		t.Fatalf("unsafe database error mapping: %#v", biz)
	}
}
