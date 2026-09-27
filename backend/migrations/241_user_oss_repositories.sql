-- Per-user object storage repositories. Secrets are stored encrypted by the application.
-- Soft-deleted rows stay so an oss-id cannot be reused by another configuration.
CREATE TABLE IF NOT EXISTS user_oss_repositories (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT       NOT NULL REFERENCES users (id),
    provider         VARCHAR(32)  NOT NULL,
    bucket           VARCHAR(512) NOT NULL,
    domain           VARCHAR(512) NOT NULL DEFAULT '',
    region           VARCHAR(128) NOT NULL DEFAULT '',
    endpoint         VARCHAR(512) NOT NULL DEFAULT '',
    access_key_id    VARCHAR(256) NOT NULL DEFAULT '',
    secret_encrypted TEXT         NOT NULL DEFAULT '',
    account_id       VARCHAR(128) NOT NULL DEFAULT '',
    force_path_style BOOLEAN      NOT NULL DEFAULT FALSE,
    bucket_url       VARCHAR(512) NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_user_oss_repositories_user_id
    ON user_oss_repositories (user_id)
    WHERE deleted_at IS NULL;

COMMENT ON TABLE user_oss_repositories IS '用户自定义对象存储库；secret_encrypted 为应用层密文，接口不回传明文';
COMMENT ON COLUMN user_oss_repositories.provider IS 'aliyun | tencent | qiniuyun | cloudflare | s3';
