package release_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

// The delivery upgrade capability issuer and verifier are a package-private
// trust boundary. This API-surface test prevents another internal package from
// gaining a public constructor that could mint or inspect the capability.
func TestDeliveryUpgradeCapabilityIsNotExported(t *testing.T) {
	packages, err := parser.ParseDir(token.NewFileSet(), ".", nil, 0)
	require.NoError(t, err)
	pkg := packages["release"]
	require.NotNil(t, pkg)

	for _, file := range pkg.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil {
				continue
			}
			require.NotEqual(t, "WithDeliveryUpgrade", function.Name.Name)
			require.NotEqual(t, "DeliveryUpgradeAuthorized", function.Name.Name)
		}
	}
}
