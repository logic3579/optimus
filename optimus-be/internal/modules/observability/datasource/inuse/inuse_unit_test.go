package inuse

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type recordingPool struct {
	query string
	args  []driver.NamedValue
	err   error
}

func (*recordingPool) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	return nil, errors.New("prepare is not expected")
}
func (*recordingPool) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, errors.New("exec is not expected")
}
func (p *recordingPool) QueryContext(_ context.Context, query string, args ...any) (*sql.Rows, error) {
	p.query = query
	p.args = make([]driver.NamedValue, len(args))
	for i, arg := range args {
		p.args[i] = driver.NamedValue{Ordinal: i + 1, Value: arg}
	}
	return nil, p.err
}
func (p *recordingPool) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	_, _ = p.QueryContext(ctx, query, args...)
	return nil
}

func counterWithPool(t *testing.T, pool *recordingPool) *Counter {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: pool}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	return New(db)
}

func TestCounterBuildsScopedReferenceQueriesAndPropagatesErrors(t *testing.T) {
	sentinel := errors.New("database unavailable")
	pool := &recordingPool{err: sentinel}
	counter := counterWithPool(t, pool)
	if counter == nil {
		t.Fatal("New returned nil")
	}
	assertQuery := func(wantTable, wantColumn string, call func() (int64, error)) {
		t.Helper()
		pool.query, pool.args = "", nil
		n, err := call()
		if n != 0 || !errors.Is(err, sentinel) {
			t.Fatalf("count=%d err=%v", n, err)
		}
		if !strings.Contains(pool.query, wantTable) || !strings.Contains(pool.query, wantColumn) {
			t.Fatalf("query %q does not contain table=%q column=%q", pool.query, wantTable, wantColumn)
		}
		if len(pool.args) == 0 || pool.args[0].Value != uint64(9) {
			t.Fatalf("query args=%#v", pool.args)
		}
	}
	ctx := context.Background()
	assertQuery("observability_datasources", "http_credential_id", func() (int64, error) {
		return counter.CountByHTTPCredentialID(ctx, 9)
	})
	assertQuery("observability_datasources", "http_credential_id", func() (int64, error) {
		return counter.CountByHTTPCredentialIDTx(ctx, counter.db, 9)
	})
	assertQuery("observability_datasources", "cluster_id", func() (int64, error) {
		return counter.CountByClusterID(ctx, 9)
	})
	assertQuery("observability_datasources", "cluster_id", func() (int64, error) {
		return counter.CountByClusterIDTx(ctx, counter.db, 9)
	})
	assertQuery("observability_panels", "datasource_id", func() (int64, error) {
		return counter.CountByDatasourceIDTx(ctx, counter.db, 9)
	})
}
