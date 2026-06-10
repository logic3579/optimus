package permissions

// Permission describes a permission code registered at startup.
type Permission struct {
	Code        string
	Name        string // i18n key
	Category    string
	Description string
}

// All P0 permission codes. Future modules append to this list.
var All = []Permission{
	// system: user
	{Code: "system:user:read", Name: "perm.system.user.read", Category: "system", Description: "Read users"},
	{Code: "system:user:write", Name: "perm.system.user.write", Category: "system", Description: "Create/update users"},
	{Code: "system:user:delete", Name: "perm.system.user.delete", Category: "system", Description: "Delete users"},
	{Code: "system:user:reset_pass", Name: "perm.system.user.reset_pass", Category: "system", Description: "Reset user password as admin"},

	// system: role
	{Code: "system:role:read", Name: "perm.system.role.read", Category: "system", Description: "Read roles"},
	{Code: "system:role:write", Name: "perm.system.role.write", Category: "system", Description: "Create/update roles and bind permissions"},
	{Code: "system:role:delete", Name: "perm.system.role.delete", Category: "system", Description: "Delete roles"},

	// system: permission
	{Code: "system:permission:read", Name: "perm.system.permission.read", Category: "system", Description: "Read permission registry"},

	// system: menu
	{Code: "system:menu:read", Name: "perm.system.menu.read", Category: "system", Description: "Read menus"},
	{Code: "system:menu:write", Name: "perm.system.menu.write", Category: "system", Description: "Create/update menus"},
	{Code: "system:menu:delete", Name: "perm.system.menu.delete", Category: "system", Description: "Delete menus"},

	// system: audit
	{Code: "system:audit:read", Name: "perm.system.audit.read", Category: "system", Description: "Read audit logs"},

	// credentials: ssh_key
	{Code: "credentials:ssh_key:read", Name: "perm.credentials.ssh_key.read", Category: "credentials", Description: "Read SSH credentials"},
	{Code: "credentials:ssh_key:write", Name: "perm.credentials.ssh_key.write", Category: "credentials", Description: "Create/update SSH credentials"},
	{Code: "credentials:ssh_key:delete", Name: "perm.credentials.ssh_key.delete", Category: "credentials", Description: "Delete SSH credentials"},
	{Code: "credentials:ssh_key:use", Name: "perm.credentials.ssh_key.use", Category: "credentials", Description: "Use SSH credentials"},

	// credentials: kubeconfig
	{Code: "credentials:kubeconfig:read", Name: "perm.credentials.kubeconfig.read", Category: "credentials", Description: "Read kubeconfigs"},
	{Code: "credentials:kubeconfig:write", Name: "perm.credentials.kubeconfig.write", Category: "credentials", Description: "Create/update kubeconfigs"},
	{Code: "credentials:kubeconfig:delete", Name: "perm.credentials.kubeconfig.delete", Category: "credentials", Description: "Delete kubeconfigs"},
	{Code: "credentials:kubeconfig:use", Name: "perm.credentials.kubeconfig.use", Category: "credentials", Description: "Use kubeconfigs"},

	// credentials: cloud_key
	{Code: "credentials:cloud_key:read", Name: "perm.credentials.cloud_key.read", Category: "credentials", Description: "Read cloud keys"},
	{Code: "credentials:cloud_key:write", Name: "perm.credentials.cloud_key.write", Category: "credentials", Description: "Create/update cloud keys"},
	{Code: "credentials:cloud_key:delete", Name: "perm.credentials.cloud_key.delete", Category: "credentials", Description: "Delete cloud keys"},
	{Code: "credentials:cloud_key:use", Name: "perm.credentials.cloud_key.use", Category: "credentials", Description: "Use cloud keys"},
}

// CodeSet returns a set for O(1) membership testing.
func CodeSet() map[string]struct{} {
	out := make(map[string]struct{}, len(All))
	for _, p := range All {
		out[p.Code] = struct{}{}
	}
	return out
}
