-- +goose Up
-- +goose StatementBegin
CREATE TABLE credentials_http_credentials (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  auth_type VARCHAR(16) NOT NULL,
  username VARCHAR(256),
  secret_ciphertext BYTEA NOT NULL,
  created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ,
  CONSTRAINT credentials_http_auth_type_check CHECK (auth_type IN ('basic', 'bearer')),
  CONSTRAINT credentials_http_username_check CHECK (
    (auth_type = 'basic' AND username IS NOT NULL AND username <> '') OR
    (auth_type = 'bearer' AND username IS NULL)
  )
);
CREATE UNIQUE INDEX credentials_http_name_unique
  ON credentials_http_credentials(name) WHERE deleted_at IS NULL;

CREATE TABLE observability_datasources (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  base_url TEXT NOT NULL,
  auth_type VARCHAR(16) NOT NULL DEFAULT 'none',
  http_credential_id BIGINT REFERENCES credentials_http_credentials(id) ON DELETE RESTRICT,
  cluster_id BIGINT REFERENCES clusters(id) ON DELETE RESTRICT,
  tls_skip_verify BOOLEAN NOT NULL DEFAULT FALSE,
  custom_ca_pem TEXT,
  description TEXT NOT NULL DEFAULT '',
  created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ,
  CONSTRAINT observability_datasource_auth_type_check CHECK (auth_type IN ('none', 'basic', 'bearer')),
  CONSTRAINT observability_datasource_credential_check CHECK (
    (auth_type = 'none' AND http_credential_id IS NULL) OR
    (auth_type IN ('basic', 'bearer') AND http_credential_id IS NOT NULL)
  )
);
CREATE UNIQUE INDEX observability_datasource_name_unique
  ON observability_datasources(name) WHERE deleted_at IS NULL;
CREATE INDEX observability_datasource_cluster_idx
  ON observability_datasources(cluster_id) WHERE deleted_at IS NULL;
CREATE INDEX observability_datasource_credential_idx
  ON observability_datasources(http_credential_id) WHERE deleted_at IS NULL;

CREATE TABLE observability_dashboards (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  refresh_interval_s INTEGER NOT NULL DEFAULT 30,
  time_range VARCHAR(16) NOT NULL DEFAULT '1h',
  created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ,
  CONSTRAINT observability_dashboard_refresh_check CHECK (refresh_interval_s BETWEEN 15 AND 3600),
  CONSTRAINT observability_dashboard_range_check CHECK (time_range IN ('15m', '1h', '6h', '24h', '7d'))
);
CREATE UNIQUE INDEX observability_dashboard_name_unique
  ON observability_dashboards(name) WHERE deleted_at IS NULL;

CREATE TABLE observability_panels (
  id BIGSERIAL PRIMARY KEY,
  dashboard_id BIGINT NOT NULL REFERENCES observability_dashboards(id) ON DELETE CASCADE,
  datasource_id BIGINT NOT NULL REFERENCES observability_datasources(id) ON DELETE RESTRICT,
  title VARCHAR(128) NOT NULL,
  panel_type VARCHAR(16) NOT NULL,
  promql TEXT NOT NULL,
  unit VARCHAR(32) NOT NULL DEFAULT 'none',
  legend VARCHAR(128) NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL,
  width SMALLINT NOT NULL DEFAULT 12,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT observability_panel_type_check CHECK (panel_type IN ('time_series', 'stat', 'table')),
  CONSTRAINT observability_panel_promql_check CHECK (length(btrim(promql)) BETWEEN 1 AND 8192),
  CONSTRAINT observability_panel_width_check CHECK (width IN (6, 12)),
  CONSTRAINT observability_panel_order_unique UNIQUE (dashboard_id, sort_order)
);
CREATE INDEX observability_panel_datasource_idx ON observability_panels(datasource_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS observability_panels;
DROP TABLE IF EXISTS observability_dashboards;
DROP TABLE IF EXISTS observability_datasources;
DROP TABLE IF EXISTS credentials_http_credentials;
-- +goose StatementEnd
