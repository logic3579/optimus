package runs

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

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

func TestServiceListDefaultsAndParsesStartedAfterOnce(t *testing.T) {
	repo := &stubListRepo{items: nil, total: 0}
	svc := NewService(repo)
	wantTime := time.Date(2026, 7, 16, 10, 11, 12, 0, time.UTC)
	result, err := svc.List(context.Background(), ListQuery{StartedAfter: wantTime.Format(time.RFC3339)})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if repo.filter.Page != 1 || repo.filter.Size != 20 {
		t.Fatalf("defaults = page %d size %d", repo.filter.Page, repo.filter.Size)
	}
	if repo.filter.StartedAfter == nil || !repo.filter.StartedAfter.Equal(wantTime) {
		t.Fatalf("parsed started_after = %v", repo.filter.StartedAfter)
	}
	if result.Items == nil {
		t.Fatal("Items must encode as an empty array, not null")
	}
}

func TestServiceListValidatesQuery(t *testing.T) {
	tests := []struct {
		name  string
		query ListQuery
	}{
		{"account beyond bigint", ListQuery{AccountID: uint64(math.MaxInt64) + 1}},
		{"invalid resource type", ListQuery{ResourceType: "bucket"}},
		{"invalid status", ListQuery{Status: "done"}},
		{"invalid timestamp", ListQuery{StartedAfter: "2026-07-16"}},
		{"negative page", ListQuery{Page: -1}},
		{"negative size", ListQuery{Size: -1}},
		{"oversize", ListQuery{Size: 201}},
		{"offset overflow", ListQuery{Page: math.MaxInt, Size: 200}},
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

func TestServiceListAcceptsDocumentedEnums(t *testing.T) {
	for _, resourceType := range []string{"", "instance", "network", "database"} {
		for _, status := range []string{"", "running", "success", "failed", "skipped"} {
			repo := &stubListRepo{}
			_, err := NewService(repo).List(context.Background(), ListQuery{ResourceType: resourceType, Status: status})
			if err != nil {
				t.Fatalf("resource_type=%q status=%q: %v", resourceType, status, err)
			}
		}
	}
}

func TestServiceListMapsRepositoryError(t *testing.T) {
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
