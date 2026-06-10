-- +goose Up
CREATE TABLE credentials_cloud_keys (
    id                     BIGSERIAL    PRIMARY KEY,
    name                   VARCHAR(128) NOT NULL UNIQUE,
    description            TEXT         NOT NULL DEFAULT '',
    provider               VARCHAR(16)  NOT NULL CHECK (provider IN ('aws','gcp','azure')),
    region                 VARCHAR(32)  NOT NULL DEFAULT '',
    access_key_id_enc      BYTEA        NOT NULL,
    secret_access_key_enc  BYTEA        NOT NULL,
    created_by_user_id     BIGINT       REFERENCES users(id) ON DELETE SET NULL,
    created_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_credentials_cloud_keys_provider   ON credentials_cloud_keys(provider);
CREATE INDEX idx_credentials_cloud_keys_created_by ON credentials_cloud_keys(created_by_user_id);

-- +goose Down
DROP TABLE credentials_cloud_keys;
