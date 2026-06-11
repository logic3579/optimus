# P0 Permissions Registry

Auto-generated from `optimus-be/internal/infra/permissions/codes.go`. Run `make dump-perms` to refresh. CI fails if this is stale.

## apps

| Code | Name (i18n) | Description |
|---|---|---|
| `apps:application:delete` | `perm.apps.application.delete` | Delete applications |
| `apps:application:read` | `perm.apps.application.read` | Read applications |
| `apps:application:write` | `perm.apps.application.write` | Create/update applications |
| `apps:release:install` | `perm.apps.release.install` | Install a helm release |
| `apps:release:rollback` | `perm.apps.release.rollback` | Rollback a helm release |
| `apps:release:uninstall` | `perm.apps.release.uninstall` | Uninstall a helm release |
| `apps:release:upgrade` | `perm.apps.release.upgrade` | Upgrade a helm release |
| `apps:repo:delete` | `perm.apps.repo.delete` | Delete chart repositories |
| `apps:repo:read` | `perm.apps.repo.read` | Read chart repositories |
| `apps:repo:write` | `perm.apps.repo.write` | Create/update chart repositories |

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

## k8s

| Code | Name (i18n) | Description |
|---|---|---|
| `k8s:cluster:read` | `perm.k8s.cluster.read` | Read clusters |
| `k8s:cluster:write` | `perm.k8s.cluster.write` | Create/update/delete clusters |
| `k8s:cluster_resource:read` | `perm.k8s.cluster_resource.read` | Read namespaces, nodes, and events |
| `k8s:config:read` | `perm.k8s.config.read` | Read configmaps |
| `k8s:log:read` | `perm.k8s.log.read` | Stream pod logs |
| `k8s:network:read` | `perm.k8s.network.read` | Read services and ingresses |
| `k8s:secret:read` | `perm.k8s.secret.read` | Read secret metadata and keys |
| `k8s:secret:reveal` | `perm.k8s.secret.reveal` | Decode secret data values |
| `k8s:workload:read` | `perm.k8s.workload.read` | Read workload resources |

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

