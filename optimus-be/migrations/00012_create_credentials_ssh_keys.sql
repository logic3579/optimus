-- +goose Up
CREATE TABLE credentials_ssh_keys (
    id                 BIGSERIAL    PRIMARY KEY,
    name               VARCHAR(128) NOT NULL UNIQUE,
    description        TEXT         NOT NULL DEFAULT '',
    username           VARCHAR(64)  NOT NULL,
    private_key_enc    BYTEA        NOT NULL,
    passphrase_enc     BYTEA,
    created_by_user_id BIGINT       REFERENCES users(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_credentials_ssh_keys_created_by ON credentials_ssh_keys(created_by_user_id);

-- +goose Down
DROP TABLE credentials_ssh_keys;
