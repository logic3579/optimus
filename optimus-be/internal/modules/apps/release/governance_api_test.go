package release_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/modules/apps/release"
)

// The verifier is the only production DeliveryUpgrade/capability API. The file
// filter deliberately excludes the in-package test-only issuer used below.
func TestDeliveryUpgradeCapabilityProductionAPISurface(t *testing.T) {
	packages, err := parser.ParseDir(token.NewFileSet(), ".", func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	require.NoError(t, err)
	pkg := packages["release"]
	require.NotNil(t, pkg)

	foundVerifier := false
	for filename, file := range pkg.Files {
		if filepath.Base(filename) != "governance.go" {
			continue
		}
		for _, declaration := range file.Decls {
			if typeDeclaration, ok := declaration.(*ast.GenDecl); ok {
				for _, spec := range typeDeclaration.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if ok && ast.IsExported(typeSpec.Name.Name) {
						require.NotContains(t, typeSpec.Name.Name, "Capability")
					}
				}
			}
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || !ast.IsExported(function.Name.Name) {
				continue
			}
			require.Equal(t, "DeliveryUpgradeAuthorized", function.Name.Name,
				"governance.go must not export capability issuers or constructors")
			foundVerifier = true
		}
	}
	require.True(t, foundVerifier)
}

func TestDeliveryUpgradeAuthorizedReadOnlyVerifier(t *testing.T) {
	ctx := release.IssueDeliveryUpgradeForTest(context.Background(), 42, "operation-1")
	require.True(t, release.DeliveryUpgradeAuthorized(ctx, 42, "operation-1", release.MutationActionUpgrade))
	require.False(t, release.DeliveryUpgradeAuthorized(ctx, 43, "operation-1", release.MutationActionUpgrade))
	require.False(t, release.DeliveryUpgradeAuthorized(ctx, 42, "operation-2", release.MutationActionUpgrade))
	require.False(t, release.DeliveryUpgradeAuthorized(ctx, 42, "operation-1", release.MutationActionInstall))
	require.False(t, release.DeliveryUpgradeAuthorized(ctx, 42, "operation-1", release.MutationActionRollback))
	require.False(t, release.DeliveryUpgradeAuthorized(ctx, 42, "operation-1", release.MutationActionUninstall))
	require.False(t, release.DeliveryUpgradeAuthorized(context.Background(), 42, "operation-1", release.MutationActionUpgrade))
}
