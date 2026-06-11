-- +goose Up
CREATE TABLE apps_chart_repos (
    id                  BIGSERIAL    PRIMARY KEY,
    name                VARCHAR(64)  NOT NULL,
    type                VARCHAR(8)   NOT NULL CHECK (type IN ('oci','http')),
    url                 TEXT         NOT NULL,
    username            VARCHAR(255) NOT NULL DEFAULT '',
    encrypted_password  BYTEA        NOT NULL DEFAULT ''::BYTEA,
    description         TEXT         NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ
);
CREATE UNIQUE INDEX apps_chart_repos_name_unique
    ON apps_chart_repos(name) WHERE deleted_at IS NULL;
CREATE INDEX apps_chart_repos_deleted_at ON apps_chart_repos(deleted_at);

CREATE TABLE apps_applications (
    id              BIGSERIAL    PRIMARY KEY,
    name            VARCHAR(64)  NOT NULL,
    cluster_id      BIGINT       NOT NULL
                    REFERENCES clusters(id) ON DELETE RESTRICT,
    namespace       VARCHAR(63)  NOT NULL,
    release_name    VARCHAR(53)  NOT NULL,
    chart_repo_id   BIGINT       NOT NULL
                    REFERENCES apps_chart_repos(id) ON DELETE RESTRICT,
    chart_name      VARCHAR(128) NOT NULL,
    description     TEXT         NOT NULL DEFAULT '',
    tags            JSONB        NOT NULL DEFAULT '[]'::JSONB,
    owner_user_id   BIGINT       REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,
    CONSTRAINT apps_applications_tags_is_array CHECK (jsonb_typeof(tags) = 'array')
);
CREATE UNIQUE INDEX apps_applications_release_unique
    ON apps_applications(cluster_id, namespace, release_name)
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX apps_applications_name_unique
    ON apps_applications(name) WHERE deleted_at IS NULL;
CREATE INDEX apps_applications_cluster_id    ON apps_applications(cluster_id);
CREATE INDEX apps_applications_owner_user_id ON apps_applications(owner_user_id);
CREATE INDEX apps_applications_deleted_at    ON apps_applications(deleted_at);

-- +goose Down
DROP TABLE IF EXISTS apps_applications;
DROP TABLE IF EXISTS apps_chart_repos;
