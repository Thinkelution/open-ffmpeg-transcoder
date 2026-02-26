CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE job_status AS ENUM (
    'pending',
    'downloading',
    'transcoding',
    'uploading',
    'completed',
    'failed',
    'cancelled'
);

CREATE TABLE jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    status job_status NOT NULL DEFAULT 'pending',
    progress REAL NOT NULL DEFAULT 0,
    speed TEXT NOT NULL DEFAULT '',
    fps REAL NOT NULL DEFAULT 0,
    eta TEXT NOT NULL DEFAULT '',

    -- Input configuration (stored as JSONB)
    input_config JSONB NOT NULL,
    -- Output configuration
    output_config JSONB NOT NULL,
    -- Transcoding settings
    settings JSONB NOT NULL,

    priority INTEGER NOT NULL DEFAULT 5,
    webhook_url TEXT NOT NULL DEFAULT '',
    metadata JSONB DEFAULT '{}',

    -- Result info
    error_message TEXT NOT NULL DEFAULT '',
    input_info JSONB DEFAULT '{}',
    output_info JSONB DEFAULT '{}',
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    duration_ms BIGINT NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_jobs_status ON jobs(status);
CREATE INDEX idx_jobs_created_at ON jobs(created_at DESC);
CREATE INDEX idx_jobs_priority ON jobs(priority DESC);
