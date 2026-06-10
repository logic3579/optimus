package cluster

import (
	"fmt"

	"k8s.io/client-go/tools/clientcmd"

	apperr "optimus-be/internal/infra/errors"
)

// validateContextAndAuth parses a kubeconfig YAML and rejects it if:
//   - it fails to parse;
//   - any AuthInfo uses an exec plugin or an auth-provider plugin;
//   - the requested context name is not present.
//
// Task 7 will promote this to an exported helper so the client.Factory can
// reuse it as defense-in-depth at run time. Keeping it unexported here for
// now preserves the package boundary.
func validateContextAndAuth(raw []byte, contextName string) error {
	apiCfg, err := clientcmd.Load(raw)
	if err != nil {
		return apperr.New(apperr.CodeValidation, "k8s.kubeconfig.invalid", err.Error())
	}
	for name, u := range apiCfg.AuthInfos {
		if u == nil {
			continue
		}
		if u.Exec != nil {
			return apperr.New(apperr.CodeValidation, "k8s.kubeconfig.exec_forbidden",
				fmt.Sprintf("user %q uses exec auth plugin", name))
		}
		if u.AuthProvider != nil {
			return apperr.New(apperr.CodeValidation, "k8s.kubeconfig.authprovider_forbidden",
				fmt.Sprintf("user %q uses auth-provider plugin", name))
		}
	}
	if _, ok := apiCfg.Contexts[contextName]; !ok {
		return apperr.New(apperr.CodeValidation, "k8s.kubeconfig.context_not_found",
			fmt.Sprintf("context %q not found", contextName))
	}
	return nil
}
