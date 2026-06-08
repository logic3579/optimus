-- +goose Up
ALTER TABLE audit_logs
    ADD CONSTRAINT fk_audit_logs_user FOREIGN KEY (user_id)
    REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE user_roles
    ADD CONSTRAINT fk_user_roles_user FOREIGN KEY (user_id)
    REFERENCES users(id) ON DELETE CASCADE,
    ADD CONSTRAINT fk_user_roles_role FOREIGN KEY (role_id)
    REFERENCES roles(id) ON DELETE CASCADE;

ALTER TABLE role_permissions
    ADD CONSTRAINT fk_role_permissions_role FOREIGN KEY (role_id)
    REFERENCES roles(id) ON DELETE CASCADE,
    ADD CONSTRAINT fk_role_permissions_permission FOREIGN KEY (permission_id)
    REFERENCES permissions(id) ON DELETE CASCADE;

ALTER TABLE refresh_tokens
    ADD CONSTRAINT fk_refresh_tokens_user FOREIGN KEY (user_id)
    REFERENCES users(id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE refresh_tokens   DROP CONSTRAINT fk_refresh_tokens_user;
ALTER TABLE role_permissions DROP CONSTRAINT fk_role_permissions_role, DROP CONSTRAINT fk_role_permissions_permission;
ALTER TABLE user_roles       DROP CONSTRAINT fk_user_roles_user, DROP CONSTRAINT fk_user_roles_role;
ALTER TABLE audit_logs       DROP CONSTRAINT fk_audit_logs_user;
