// Package migrations exposes the SQL migration files as an embed.FS so they
// can be applied programmatically from cmd/migrate (and from tests) without
// needing the migrations/ directory to exist on disk at runtime.
//
// The existing Makefile target `migrate-up` still works against the on-disk
// directory; this package coexists with that path.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
