-- +goose Up
-- +goose StatementBegin
CREATE TABLE cloud_accounts (
  id              BIGSERIAL PRIMARY KEY,
  name            VARCHAR(128) NOT NULL,
  provider        VARCHAR(16)  NOT NULL,
  cloudkey_id     BIGINT       NOT NULL REFERENCES credentials_cloud_keys(id),
  enabled_regions TEXT[]       NOT NULL,
  enabled         BOOLEAN      NOT NULL DEFAULT true,
  description     TEXT         NOT NULL DEFAULT '',
  created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
  deleted_at      TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_cloud_accounts_name_alive
    ON cloud_accounts(name) WHERE deleted_at IS NULL;
CREATE INDEX idx_cloud_accounts_cloudkey ON cloud_accounts(cloudkey_id);

CREATE TABLE aws_instances (
  id                BIGSERIAL PRIMARY KEY,
  cloud_account_id  BIGINT       NOT NULL REFERENCES cloud_accounts(id),
  region            VARCHAR(32)  NOT NULL,
  instance_id       VARCHAR(32)  NOT NULL,
  name              TEXT         NOT NULL DEFAULT '',
  instance_type     VARCHAR(32)  NOT NULL DEFAULT '',
  state             VARCHAR(16)  NOT NULL DEFAULT '',
  private_ip        INET,
  public_ip         INET,
  vpc_id            VARCHAR(32)  NOT NULL DEFAULT '',
  subnet_id         VARCHAR(32)  NOT NULL DEFAULT '',
  availability_zone VARCHAR(32)  NOT NULL DEFAULT '',
  launch_time       TIMESTAMPTZ,
  tags              JSONB        NOT NULL DEFAULT '{}'::jsonb,
  last_seen_at      TIMESTAMPTZ  NOT NULL,
  created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
  deleted_at        TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_aws_instances_keytuple
    ON aws_instances(cloud_account_id, region, instance_id);
CREATE INDEX idx_aws_instances_vpc        ON aws_instances(vpc_id);
CREATE INDEX idx_aws_instances_private_ip ON aws_instances(private_ip);
CREATE INDEX idx_aws_instances_tags_gin   ON aws_instances USING GIN (tags);
CREATE INDEX idx_aws_instances_last_seen  ON aws_instances(last_seen_at);

CREATE TABLE aws_vpcs (
  id                BIGSERIAL PRIMARY KEY,
  cloud_account_id  BIGINT       NOT NULL REFERENCES cloud_accounts(id),
  region            VARCHAR(32)  NOT NULL,
  vpc_id            VARCHAR(32)  NOT NULL,
  name              TEXT         NOT NULL DEFAULT '',
  cidr_block        CIDR,
  is_default        BOOLEAN      NOT NULL DEFAULT false,
  state             VARCHAR(16)  NOT NULL DEFAULT '',
  tags              JSONB        NOT NULL DEFAULT '{}'::jsonb,
  last_seen_at      TIMESTAMPTZ  NOT NULL,
  created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
  deleted_at        TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_aws_vpcs_keytuple ON aws_vpcs(cloud_account_id, region, vpc_id);
CREATE INDEX idx_aws_vpcs_last_seen      ON aws_vpcs(last_seen_at);

CREATE TABLE aws_subnets (
  id                BIGSERIAL PRIMARY KEY,
  cloud_account_id  BIGINT       NOT NULL REFERENCES cloud_accounts(id),
  region            VARCHAR(32)  NOT NULL,
  subnet_id         VARCHAR(32)  NOT NULL,
  vpc_id            VARCHAR(32)  NOT NULL,
  cidr_block        CIDR,
  availability_zone VARCHAR(32)  NOT NULL DEFAULT '',
  name              TEXT         NOT NULL DEFAULT '',
  tags              JSONB        NOT NULL DEFAULT '{}'::jsonb,
  last_seen_at      TIMESTAMPTZ  NOT NULL,
  created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
  deleted_at        TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_aws_subnets_keytuple ON aws_subnets(cloud_account_id, region, subnet_id);
CREATE INDEX idx_aws_subnets_vpc            ON aws_subnets(vpc_id);
CREATE INDEX idx_aws_subnets_last_seen      ON aws_subnets(last_seen_at);

CREATE TABLE aws_databases (
  id                  BIGSERIAL PRIMARY KEY,
  cloud_account_id    BIGINT       NOT NULL REFERENCES cloud_accounts(id),
  region              VARCHAR(32)  NOT NULL,
  db_instance_id      VARCHAR(64)  NOT NULL,
  engine              VARCHAR(32)  NOT NULL DEFAULT '',
  engine_version      VARCHAR(32)  NOT NULL DEFAULT '',
  instance_class      VARCHAR(32)  NOT NULL DEFAULT '',
  status              VARCHAR(32)  NOT NULL DEFAULT '',
  endpoint            TEXT         NOT NULL DEFAULT '',
  port                INTEGER,
  multi_az            BOOLEAN      NOT NULL DEFAULT false,
  publicly_accessible BOOLEAN      NOT NULL DEFAULT false,
  storage_gb          INTEGER,
  tags                JSONB        NOT NULL DEFAULT '{}'::jsonb,
  last_seen_at        TIMESTAMPTZ  NOT NULL,
  created_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
  deleted_at          TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_aws_databases_keytuple
    ON aws_databases(cloud_account_id, region, db_instance_id);
CREATE INDEX idx_aws_databases_last_seen ON aws_databases(last_seen_at);

CREATE TABLE assets_sync_runs (
  id                   BIGSERIAL PRIMARY KEY,
  cloud_account_id     BIGINT       NOT NULL REFERENCES cloud_accounts(id),
  region               VARCHAR(32)  NOT NULL,
  resource_type        VARCHAR(16)  NOT NULL,
  started_at           TIMESTAMPTZ  NOT NULL,
  finished_at          TIMESTAMPTZ,
  status               VARCHAR(16)  NOT NULL,
  items_seen           INTEGER      NOT NULL DEFAULT 0,
  items_softdeleted    INTEGER      NOT NULL DEFAULT 0,
  error                TEXT         NOT NULL DEFAULT '',
  error_code           INTEGER      NOT NULL DEFAULT 0,
  trigger              VARCHAR(16)  NOT NULL,
  triggered_by_user_id BIGINT       REFERENCES users(id),
  created_at           TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX idx_assets_sync_runs_account_type
    ON assets_sync_runs(cloud_account_id, resource_type, started_at DESC);
CREATE INDEX idx_assets_sync_runs_status
    ON assets_sync_runs(status, started_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS assets_sync_runs;
DROP TABLE IF EXISTS aws_databases;
DROP TABLE IF EXISTS aws_subnets;
DROP TABLE IF EXISTS aws_vpcs;
DROP TABLE IF EXISTS aws_instances;
DROP TABLE IF EXISTS cloud_accounts;
-- +goose StatementEnd
