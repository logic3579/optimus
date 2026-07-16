package database

import (
	"context"
	"errors"
	"math"
	"testing"

	apperr "optimus-be/internal/infra/errors"
)

type stubRepository struct {
	items  []Summary
	total  int64
	err    error
	filter ListFilter
}

func (r *stubRepository) List(_ context.Context, filter ListFilter) ([]Summary, int64, error) {
	r.filter = filter
	return r.items, r.total, r.err
}

func TestServiceListDefaultsAndReturnsNonNilItems(t *testing.T) {
	repo := &stubRepository{}
	result, err := NewService(repo).List(context.Background(), ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if repo.filter.Page != 1 || repo.filter.Size != 20 {
		t.Fatalf("defaults = page %d size %d", repo.filter.Page, repo.filter.Size)
	}
	if result.Items == nil {
		t.Fatal("items must be an empty JSON array")
	}
}

func TestServiceListValidatesDatabaseAndPaginationRanges(t *testing.T) {
	for _, test := range []struct {
		name  string
		query ListQuery
	}{
		{"account beyond bigint", ListQuery{AccountID: uint64(math.MaxInt64) + 1}},
		{"negative page", ListQuery{Page: -1}},
		{"negative size", ListQuery{Size: -1}},
		{"oversize", ListQuery{Size: 201}},
		{"offset overflow", ListQuery{Page: math.MaxInt, Size: 200}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewService(&stubRepository{}).List(context.Background(), test.query)
			var biz *apperr.BizError
			if !errors.As(err, &biz) || biz.Code != apperr.CodeValidation {
				t.Fatalf("error = %#v, want validation error", err)
			}
		})
	}
}

func TestServiceListPassesAllFilters(t *testing.T) {
	repo := &stubRepository{}
	query := ListQuery{AccountID: 7, Region: "us-east-1", Engine: "postgres", Status: "available", Q: "orders", IncludeDeleted: true, Page: 2, Size: 50}
	if _, err := NewService(repo).List(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	want := ListFilter{AccountID: 7, Region: "us-east-1", Engine: "postgres", Status: "available", Q: "orders", IncludeDeleted: true, Page: 2, Size: 50}
	if repo.filter != want {
		t.Fatalf("filter = %#v, want %#v", repo.filter, want)
	}
}

func TestServiceListMapsRepositoryErrorsSafely(t *testing.T) {
	secret := errors.New("dsn password=do-not-leak")
	_, err := NewService(&stubRepository{err: secret}).List(context.Background(), ListQuery{})
	var biz *apperr.BizError
	if !errors.As(err, &biz) || biz.Code != apperr.CodeDBError {
		t.Fatalf("error = %#v", err)
	}
	if biz.Message != "database operation failed" || biz.MessageKey != "common.database_error" {
		t.Fatalf("unsafe error = %#v", biz)
	}
}
