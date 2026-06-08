-- +goose Up
CREATE TABLE menus (
    id              BIGSERIAL PRIMARY KEY,
    parent_id       BIGINT,
    code            VARCHAR(64)  NOT NULL,
    name            VARCHAR(128) NOT NULL,
    path            VARCHAR(255) NOT NULL DEFAULT '',
    component       VARCHAR(255) NOT NULL DEFAULT '',
    icon            VARCHAR(64)  NOT NULL DEFAULT '',
    permission_code VARCHAR(128),
    sort_order      INT          NOT NULL DEFAULT 0,
    hidden          BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX idx_menus_parent_id  ON menus (parent_id);
CREATE INDEX idx_menus_deleted_at ON menus (deleted_at);

-- +goose Down
DROP TABLE menus;
