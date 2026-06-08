-- +goose Up
CREATE UNIQUE INDEX users_username_uniq ON users (username) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX users_email_uniq    ON users (email)    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX roles_code_uniq     ON roles (code)     WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX menus_code_uniq     ON menus (code)     WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX users_username_uniq;
DROP INDEX users_email_uniq;
DROP INDEX roles_code_uniq;
DROP INDEX menus_code_uniq;
