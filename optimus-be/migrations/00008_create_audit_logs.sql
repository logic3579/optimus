-- +goose Up
CREATE TABLE audit_logs (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT,
    action       VARCHAR(64)  NOT NULL,
    target_type  VARCHAR(64)  NOT NULL DEFAULT '',
    target_id    VARCHAR(64)  NOT NULL DEFAULT '',
    payload      JSONB        NOT NULL DEFAULT '{}'::JSONB,
    ip           VARCHAR(64)  NOT NULL DEFAULT '',
    user_agent   VARCHAR(512) NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_user_id     ON audit_logs (user_id);
CREATE INDEX idx_audit_logs_action      ON audit_logs (action);
CREATE INDEX idx_audit_logs_created_at  ON audit_logs (created_at);

-- +goose Down
DROP TABLE audit_logs;
