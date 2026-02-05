-- 0001_init.sql
-- Initial schema for gionx state store (SQLite).

CREATE TABLE IF NOT EXISTS schema_migrations (
  id TEXT PRIMARY KEY,
  applied_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  root_path TEXT NOT NULL,
  repo_pool_path TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS workspaces (
  id TEXT PRIMARY KEY,
  generation INTEGER NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
  description TEXT NOT NULL,
  source_url TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  archived_commit_sha TEXT,
  reopened_commit_sha TEXT
);

CREATE TABLE IF NOT EXISTS repos (
  repo_uid TEXT PRIMARY KEY,
  repo_key TEXT NOT NULL,
  remote_url TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS workspace_repos (
  workspace_id TEXT NOT NULL,
  repo_uid TEXT NOT NULL,
  repo_key TEXT NOT NULL,
  alias TEXT NOT NULL,
  branch TEXT NOT NULL,
  base_ref TEXT NOT NULL,
  repo_spec_input TEXT NOT NULL,
  missing_at INTEGER,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (workspace_id, repo_uid),
  UNIQUE (workspace_id, alias),
  FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
  FOREIGN KEY (repo_uid) REFERENCES repos(repo_uid)
);

CREATE TABLE IF NOT EXISTS workspace_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id TEXT NOT NULL,
  workspace_generation INTEGER NOT NULL,
  event_type TEXT NOT NULL CHECK (event_type IN ('created', 'archived', 'reopened', 'purged')),
  at INTEGER NOT NULL,
  meta TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_workspace_events_workspace_gen_at
  ON workspace_events (workspace_id, workspace_generation, at);

CREATE INDEX IF NOT EXISTS idx_workspace_repos_workspace_id
  ON workspace_repos (workspace_id);

CREATE INDEX IF NOT EXISTS idx_workspace_repos_repo_uid
  ON workspace_repos (repo_uid);

CREATE INDEX IF NOT EXISTS idx_repos_repo_key
  ON repos (repo_key);
