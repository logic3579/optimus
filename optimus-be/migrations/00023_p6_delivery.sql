-- +goose Up
-- +goose StatementBegin
CREATE TABLE delivery_projects (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  owner_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ,
  CONSTRAINT delivery_projects_name_check CHECK (length(btrim(name)) BETWEEN 1 AND 128)
);
CREATE UNIQUE INDEX delivery_projects_active_name_unique
  ON delivery_projects(name) WHERE deleted_at IS NULL;

CREATE TABLE delivery_environments (
  id BIGSERIAL PRIMARY KEY,
  project_id BIGINT NOT NULL REFERENCES delivery_projects(id) ON DELETE RESTRICT,
  environment_key VARCHAR(64) NOT NULL,
  display_name VARCHAR(128) NOT NULL,
  application_id BIGINT NOT NULL REFERENCES apps_applications(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ,
  CONSTRAINT delivery_environments_key_check CHECK (length(btrim(environment_key)) BETWEEN 1 AND 64),
  CONSTRAINT delivery_environments_display_name_check CHECK (length(btrim(display_name)) BETWEEN 1 AND 128)
);
CREATE UNIQUE INDEX delivery_environments_active_project_key_unique
  ON delivery_environments(project_id, environment_key) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX delivery_environments_active_application_unique
  ON delivery_environments(application_id) WHERE deleted_at IS NULL;

CREATE TABLE delivery_pipelines (
  id BIGSERIAL PRIMARY KEY,
  project_id BIGINT NOT NULL REFERENCES delivery_projects(id) ON DELETE RESTRICT,
  version INTEGER NOT NULL,
  created_by_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  published_at TIMESTAMPTZ NOT NULL,
  is_current BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT delivery_pipelines_version_check CHECK (version > 0),
  CONSTRAINT delivery_pipelines_project_version_unique UNIQUE (project_id, version)
);
CREATE UNIQUE INDEX delivery_pipelines_current_project_unique
  ON delivery_pipelines(project_id) WHERE is_current;

CREATE TABLE delivery_pipeline_stages (
  id BIGSERIAL PRIMARY KEY,
  pipeline_id BIGINT NOT NULL REFERENCES delivery_pipelines(id) ON DELETE CASCADE,
  environment_id BIGINT NOT NULL REFERENCES delivery_environments(id) ON DELETE RESTRICT,
  stage_order INTEGER NOT NULL,
  executor VARCHAR(64) NOT NULL,
  approval_required BOOLEAN NOT NULL DEFAULT FALSE,
  timeout_seconds INTEGER NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT delivery_pipeline_stages_order_check CHECK (stage_order > 0),
  CONSTRAINT delivery_pipeline_stages_executor_check CHECK (executor IN ('helm_upgrade_existing_release')),
  CONSTRAINT delivery_pipeline_stages_timeout_check CHECK (timeout_seconds BETWEEN 1 AND 86400),
  CONSTRAINT delivery_pipeline_stages_pipeline_order_unique UNIQUE (pipeline_id, stage_order),
  CONSTRAINT delivery_pipeline_stages_pipeline_environment_unique UNIQUE (pipeline_id, environment_id)
);

CREATE TABLE delivery_runs (
  id BIGSERIAL PRIMARY KEY,
  project_id BIGINT NOT NULL REFERENCES delivery_projects(id) ON DELETE RESTRICT,
  pipeline_id BIGINT NOT NULL REFERENCES delivery_pipelines(id) ON DELETE RESTRICT,
  pipeline_version INTEGER NOT NULL,
  chart_repo_id BIGINT NOT NULL REFERENCES apps_chart_repos(id) ON DELETE RESTRICT,
  chart_name VARCHAR(128) NOT NULL,
  chart_version VARCHAR(128) NOT NULL,
  chart_digest VARCHAR(128) NOT NULL,
  initiator_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  idempotency_key VARCHAR(128) NOT NULL,
  request_fingerprint VARCHAR(128) NOT NULL,
  state VARCHAR(32) NOT NULL,
  retry_of_run_id BIGINT REFERENCES delivery_runs(id) ON DELETE RESTRICT,
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  error_code INTEGER,
  error_message_key VARCHAR(128),
  correlation_id VARCHAR(128),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT delivery_runs_pipeline_version_check CHECK (pipeline_version > 0),
  CONSTRAINT delivery_runs_state_check CHECK (state IN (
    'queued', 'running', 'waiting_approval', 'cancel_requested', 'reconciling',
    'succeeded', 'failed', 'rejected', 'canceled', 'timed_out', 'outcome_unknown'
  )),
  CONSTRAINT delivery_runs_idempotency_unique UNIQUE (project_id, initiator_user_id, idempotency_key)
);
CREATE UNIQUE INDEX delivery_runs_active_project_unique
  ON delivery_runs(project_id)
  WHERE state IN ('queued', 'running', 'waiting_approval', 'cancel_requested', 'reconciling', 'outcome_unknown');

CREATE TABLE delivery_run_stages (
  id BIGSERIAL PRIMARY KEY,
  run_id BIGINT NOT NULL REFERENCES delivery_runs(id) ON DELETE CASCADE,
  environment_id BIGINT NOT NULL REFERENCES delivery_environments(id) ON DELETE RESTRICT,
  environment_key VARCHAR(64) NOT NULL,
  environment_name VARCHAR(128) NOT NULL,
  application_id BIGINT NOT NULL REFERENCES apps_applications(id) ON DELETE RESTRICT,
  cluster_id BIGINT NOT NULL REFERENCES clusters(id) ON DELETE RESTRICT,
  namespace VARCHAR(63) NOT NULL,
  release_name VARCHAR(53) NOT NULL,
  stage_order INTEGER NOT NULL,
  executor VARCHAR(64) NOT NULL,
  approval_required BOOLEAN NOT NULL DEFAULT FALSE,
  timeout_seconds INTEGER NOT NULL,
  state VARCHAR(32) NOT NULL,
  operation_id VARCHAR(64) NOT NULL,
  lease_owner VARCHAR(128),
  lease_expires_at TIMESTAMPTZ,
  result_revision BIGINT,
  result_digest VARCHAR(128),
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  error_code INTEGER,
  error_message_key VARCHAR(128),
  correlation_id VARCHAR(128),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT delivery_run_stages_order_check CHECK (stage_order > 0),
  CONSTRAINT delivery_run_stages_executor_check CHECK (executor IN ('helm_upgrade_existing_release')),
  CONSTRAINT delivery_run_stages_timeout_check CHECK (timeout_seconds BETWEEN 1 AND 86400),
  CONSTRAINT delivery_run_stages_state_check CHECK (state IN (
    'pending', 'waiting_approval', 'queued', 'running', 'reconciling',
    'succeeded', 'failed', 'rejected', 'canceled', 'timed_out', 'outcome_unknown'
  )),
  CONSTRAINT delivery_run_stages_id_run_unique UNIQUE (id, run_id),
  CONSTRAINT delivery_run_stages_run_order_unique UNIQUE (run_id, stage_order),
  CONSTRAINT delivery_run_stages_operation_unique UNIQUE (operation_id)
);

CREATE TABLE delivery_approvals (
  id BIGSERIAL PRIMARY KEY,
  run_id BIGINT NOT NULL REFERENCES delivery_runs(id) ON DELETE CASCADE,
  run_stage_id BIGINT NOT NULL,
  requested_at TIMESTAMPTZ NOT NULL,
  decision VARCHAR(16) NOT NULL DEFAULT 'pending',
  decided_by_user_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  comment VARCHAR(512) NOT NULL DEFAULT '',
  decided_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT delivery_approvals_decision_check CHECK (decision IN ('pending', 'approved', 'rejected')),
  CONSTRAINT delivery_approvals_stage_run_fk FOREIGN KEY (run_stage_id, run_id)
    REFERENCES delivery_run_stages(id, run_id) ON DELETE CASCADE,
  CONSTRAINT delivery_approvals_stage_unique UNIQUE (run_stage_id),
  CONSTRAINT delivery_approvals_decision_fields_check CHECK (
    (decision = 'pending' AND decided_by_user_id IS NULL AND decided_at IS NULL AND comment = '') OR
    (decision IN ('approved', 'rejected') AND decided_by_user_id IS NOT NULL AND decided_at IS NOT NULL AND length(btrim(comment)) BETWEEN 1 AND 512)
  )
);

CREATE TABLE delivery_run_events (
  id BIGSERIAL PRIMARY KEY,
  run_id BIGINT NOT NULL REFERENCES delivery_runs(id) ON DELETE CASCADE,
  run_stage_id BIGINT,
  event_type VARCHAR(64) NOT NULL,
  old_state VARCHAR(32),
  new_state VARCHAR(32),
  actor_type VARCHAR(16) NOT NULL,
  actor_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  error_code INTEGER,
  error_message_key VARCHAR(128),
  correlation_id VARCHAR(128),
  metadata JSONB NOT NULL DEFAULT '{}'::JSONB,
  CONSTRAINT delivery_run_events_type_check CHECK (length(btrim(event_type)) BETWEEN 1 AND 64),
  CONSTRAINT delivery_run_events_old_state_check CHECK (old_state IS NULL OR old_state IN (
    'pending', 'queued', 'running', 'waiting_approval', 'cancel_requested', 'reconciling',
    'succeeded', 'failed', 'rejected', 'canceled', 'timed_out', 'outcome_unknown'
  )),
  CONSTRAINT delivery_run_events_new_state_check CHECK (new_state IS NULL OR new_state IN (
    'pending', 'queued', 'running', 'waiting_approval', 'cancel_requested', 'reconciling',
    'succeeded', 'failed', 'rejected', 'canceled', 'timed_out', 'outcome_unknown'
  )),
  CONSTRAINT delivery_run_events_stage_run_fk FOREIGN KEY (run_stage_id, run_id)
    REFERENCES delivery_run_stages(id, run_id) ON DELETE CASCADE,
  CONSTRAINT delivery_run_events_actor_type_check CHECK (actor_type IN ('system', 'user')),
  CONSTRAINT delivery_run_events_metadata_check CHECK (
    jsonb_typeof(metadata) = 'object' AND octet_length(metadata::TEXT) <= 4096
  )
);
CREATE INDEX delivery_run_events_run_id_id_idx ON delivery_run_events(run_id, id);

CREATE TABLE apps_release_operations (
  id BIGSERIAL PRIMARY KEY,
  application_id BIGINT NOT NULL REFERENCES apps_applications(id) ON DELETE RESTRICT,
  operation_id VARCHAR(64) NOT NULL,
  kind VARCHAR(64) NOT NULL,
  state VARCHAR(16) NOT NULL,
  lease_owner VARCHAR(128),
  lease_expires_at TIMESTAMPTZ,
  result_revision BIGINT,
  result_digest VARCHAR(128),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  finished_at TIMESTAMPTZ,
  CONSTRAINT apps_release_operations_kind_check CHECK (length(btrim(kind)) BETWEEN 1 AND 64),
  CONSTRAINT apps_release_operations_state_check CHECK (state IN ('active', 'succeeded', 'failed', 'reconciling')),
  CONSTRAINT apps_release_operations_operation_unique UNIQUE (operation_id)
);
CREATE UNIQUE INDEX apps_release_operations_active_application_unique
  ON apps_release_operations(application_id) WHERE state IN ('active', 'reconciling');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS apps_release_operations;
DROP TABLE IF EXISTS delivery_run_events;
DROP TABLE IF EXISTS delivery_approvals;
DROP TABLE IF EXISTS delivery_run_stages;
DROP TABLE IF EXISTS delivery_runs;
DROP TABLE IF EXISTS delivery_pipeline_stages;
DROP TABLE IF EXISTS delivery_pipelines;
DROP TABLE IF EXISTS delivery_environments;
DROP TABLE IF EXISTS delivery_projects;
-- +goose StatementEnd
