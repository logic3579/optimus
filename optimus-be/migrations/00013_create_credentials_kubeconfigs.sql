-- +goose Up
CREATE TABLE credentials_kubeconfigs (
    id                 BIGSERIAL    PRIMARY KEY,
    name               VARCHAR(128) NOT NULL UNIQUE,
    description        TEXT         NOT NULL DEFAULT '',
    default_namespace  VARCHAR(64)  NOT NULL DEFAULT '',
    kubeconfig_enc     BYTEA        NOT NULL,
    created_by_user_id BIGINT       REFERENCES users(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_credentials_kubeconfigs_created_by ON credentials_kubeconfigs(created_by_user_id);

-- +goose Down
DROP TABLE credentials_kubeconfigs;
