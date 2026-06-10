-- +goose Up
CREATE TABLE clusters (
    id              BIGSERIAL    PRIMARY KEY,
    name            VARCHAR(64)  NOT NULL,
    kubeconfig_id   BIGINT       NOT NULL
                    REFERENCES credentials_kubeconfigs(id) ON DELETE RESTRICT,
    context         VARCHAR(128) NOT NULL,
    description     TEXT         NOT NULL DEFAULT '',
    tags            JSONB        NOT NULL DEFAULT '[]'::JSONB,
    last_health_at  TIMESTAMPTZ,
    last_health_ok  BOOLEAN,
    last_health_msg TEXT         NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,
    CONSTRAINT clusters_tags_is_array
        CHECK (jsonb_typeof(tags) = 'array')
);

CREATE UNIQUE INDEX clusters_name_unique
    ON clusters(name) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX clusters_kubeconfig_context_unique
    ON clusters(kubeconfig_id, context) WHERE deleted_at IS NULL;
CREATE INDEX clusters_kubeconfig_id_idx
    ON clusters(kubeconfig_id) WHERE deleted_at IS NULL;
CREATE INDEX clusters_tags_gin
    ON clusters USING GIN (tags) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE clusters;
