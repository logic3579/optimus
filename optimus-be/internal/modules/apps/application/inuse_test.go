package application_test

import (
	"optimus-be/internal/modules/apps/application"
)

// This file's primary value is the compile-time conformance check that
// *application.Repo satisfies the application.Counter interface. The runtime
// behaviour of CountBy* is exercised in repo_test.go (which requires the
// dbtest tag); this assertion lives outside the build-tag so the contract is
// enforced even on a CI build without dockertest available.
var _ application.Counter = (*application.Repo)(nil)
