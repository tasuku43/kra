-- 0002_repo_usage_daily.sql
-- Lightweight daily usage counters for repo selection ranking.

CREATE TABLE IF NOT EXISTS repo_usage_daily (
  repo_uid TEXT NOT NULL,
  day INTEGER NOT NULL, -- local date key: YYYYMMDD
  add_count INTEGER NOT NULL DEFAULT 0,
  last_added_at INTEGER NOT NULL,
  PRIMARY KEY (repo_uid, day),
  FOREIGN KEY (repo_uid) REFERENCES repos(repo_uid) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_repo_usage_daily_day
  ON repo_usage_daily (day);

