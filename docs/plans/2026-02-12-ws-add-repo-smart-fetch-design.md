# ws add-repo smart-fetch design (GiönX)

## Background

Giön は worktree 作成時にリモート最新化を重視している。一方で GiönX の `ws add-repo` は、repo pool にある bare repo のローカル参照を前提に worktree を作るため、直前のリモート更新が保証されない。  
本設計は、Giön の安全性を取り込みつつ、GiönX の操作テンポを維持するために `smart-fetch` を導入する。

## Decision Summary

- fetch policy: `smart-fetch`（TTL ベース）
- default TTL: `5m`
- fetch failure policy: `条件付き`
  - 認証/権限/到達不能: 中断（fail-fast）
  - 一時的エラー: 1 回だけ再試行し、失敗なら中断
- user flags:
  - `--refresh`: TTL 無視で fetch 強制
  - `--no-fetch`: fetch 判定と fetch 実行を無効化

## Scope

- 対象: `gionx ws add-repo`（human/json の両モード）
- 非対象:
  - `repo pool add`（現行どおり常時 fetch）
  - `ws reopen`（現行どおり復元時 fetch）

## UX / CLI Spec

`ws add-repo` 実行時、repo ごとに次を判定する。

1. `--no-fetch` が指定された場合: fetch 判定をスキップ
2. `--refresh` が指定された場合: fetch を必ず実行
3. 上記以外: `last_fetched_at` と TTL(5m) で判定
4. ただし TTL 内でも以下は fetch 強制:
   - 指定 `base_ref` が bare に存在しない
   - 指定 branch の remote ref が bare に存在しない

plan 表示に fetch 方針を明示する。

- `fetch: skipped (fresh, age=2m13s <= 5m)`
- `fetch: required (stale, age=18m)`
- `fetch: required (--refresh)`
- `fetch: skipped (--no-fetch)`

## Data Model

state store の repos テーブルに次を追加する。

- `last_fetched_at INTEGER NULL` (unix seconds, UTC)
- 既存データは `NULL` 開始（初回は stale 扱いで fetch）

DB 未使用/不可時は fallback を持つ。

- bare repo 配下に `.gionx-fetch-meta.json` を保存
  - `{ "last_fetched_at": 1739356800 }`
- 更新は fetch 成功時のみ

## Execution Flow (per repo)

1. candidate 取得
2. fetch 必要性判定（flags / TTL / ref existence）
3. 必要なら `git fetch origin --prune` 実行
4. 成功時 `last_fetched_at` 更新
5. `base_ref` / branch existence 再評価
6. local branch 作成（必要時）
7. `worktree add`

## Error Handling Policy

fetch エラー分類:

- fatal:
  - auth/permission (401/403, authentication failed, permission denied)
  - remote not reachable (name resolution, timeout, network unreachable)
  - repository not found
  - 結果: 即時中断
- retryable:
  - transient transport failure
  - 結果: 1 回再試行、失敗なら中断

`--no-fetch` 時は fetch に起因するエラーは出ないが、後続で `base_ref not found` 等が出た場合は既存ルールで中断。

## Implementation Notes

- 既存の `preflightAddRepoPlan` は ref 存在チェックを行う。  
  fetch 後に再チェックする責務を明確化し、plan 生成時点と apply 直前で一貫性を持たせる。
- 並列 fetch は将来拡張。初期実装は逐次でよい（ロールバック簡潔性を優先）。
- debug log に判定理由を必ず記録:
  - `fetch decision repo=<repo> reason=stale age=... ttl=...`
  - `fetch decision repo=<repo> reason=no-fetch-flag`

## Test Plan

unit:

- TTL 判定:
  - `last_fetched_at=NULL` -> required
  - age < 5m -> skip
  - age >= 5m -> required
- flags:
  - `--refresh` always required
  - `--no-fetch` always skip（強制条件より優先）
- 強制 fetch 条件:
  - base_ref missing -> required
  - remote branch missing -> required
- error classification:
  - fatal / retryable の判定と分岐

integration:

- fresh cache で fetch 省略し worktree 作成成功
- stale cache で fetch 後に worktree 作成成功
- `--refresh` で常に fetch
- `--no-fetch` で fetch せず、missing ref なら適切に失敗
- retryable エラー 1 回失敗 -> 再試行成功
- fatal エラー -> 即中断

## Rollout

1. DB migration（`last_fetched_at`）
2. 判定ロジック + fetch 実行 + timestamp 更新
3. plan 表示/JSON への fetch decision 反映
4. テスト追加
5. ドキュメント更新（`ws add-repo` spec）
