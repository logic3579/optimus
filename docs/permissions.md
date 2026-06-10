# P0 Permissions Registry

Auto-generated from `optimus-be/internal/infra/permissions/codes.go`. Run `make dump-perms` to refresh. CI fails if this is stale.

## credentials

| Code | Name (i18n) | Description |
|---|---|---|
| `credentials:cloud_key:delete` | `perm.credentials.cloud_key.delete` | Delete cloud keys |
| `credentials:cloud_key:read` | `perm.credentials.cloud_key.read` | Read cloud keys |
| `credentials:cloud_key:use` | `perm.credentials.cloud_key.use` | Use cloud keys |
| `credentials:cloud_key:write` | `perm.credentials.cloud_key.write` | Create/update cloud keys |
| `credentials:kubeconfig:delete` | `perm.credentials.kubeconfig.delete` | Delete kubeconfigs |
| `credentials:kubeconfig:read` | `perm.credentials.kubeconfig.read` | Read kubeconfigs |
| `credentials:kubeconfig:use` | `perm.credentials.kubeconfig.use` | Use kubeconfigs |
| `credentials:kubeconfig:write` | `perm.credentials.kubeconfig.write` | Create/update kubeconfigs |
| `credentials:ssh_key:delete` | `perm.credentials.ssh_key.delete` | Delete SSH credentials |
| `credentials:ssh_key:read` | `perm.credentials.ssh_key.read` | Read SSH credentials |
| `credentials:ssh_key:use` | `perm.credentials.ssh_key.use` | Use SSH credentials |
| `credentials:ssh_key:write` | `perm.credentials.ssh_key.write` | Create/update SSH credentials |

## system

| Code | Name (i18n) | Description |
|---|---|---|
| `system:audit:read` | `perm.system.audit.read` | Read audit logs |
| `system:menu:delete` | `perm.system.menu.delete` | Delete menus |
| `system:menu:read` | `perm.system.menu.read` | Read menus |
| `system:menu:write` | `perm.system.menu.write` | Create/update menus |
| `system:permission:read` | `perm.system.permission.read` | Read permission registry |
| `system:role:delete` | `perm.system.role.delete` | Delete roles |
| `system:role:read` | `perm.system.role.read` | Read roles |
| `system:role:write` | `perm.system.role.write` | Create/update roles and bind permissions |
| `system:user:delete` | `perm.system.user.delete` | Delete users |
| `system:user:read` | `perm.system.user.read` | Read users |
| `system:user:reset_pass` | `perm.system.user.reset_pass` | Reset user password as admin |
| `system:user:write` | `perm.system.user.write` | Create/update users |

