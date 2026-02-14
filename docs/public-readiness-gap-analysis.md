---
title: "公開前ギャップ分析（Public Readiness Gap Analysis）"
status: proposed
date: 2026-02-14
---

# 公開前ギャップ分析（2026-02-14）

## 1. 目的

`kra` を外部公開する前提で、現状実装と公開運用に必要な要件の差分を整理する。

このドキュメントは「今すぐ公開して問題ないか」の判断材料として、
不足点を優先度付きで明確化することを目的とする。

## 2. 調査範囲

- 実装面: CLI コマンド群、仕様/バックログ整合、品質ゲート
- 公開運用面: README、OSS 運営ドキュメント、CI/リリース導線
- 自動化適性: JSON 契約、非対話モード、配布/バージョン方針

## 3. 主要な所見（結論）

現状は「内部開発向けとしては高品質」だが、
「OSS 公開運用としては未整備項目が多く、公開 Ready ではない」。

特に P0（公開前必須）を先に解消しないと、
初見ユーザーが導入・評価・貢献を継続できないリスクが高い。

## 4. 優先度付きギャップ一覧

### P0: 公開前に必須

1. README が開発者向け最小記述に留まり、公開導線として不十分
   - 根拠: `README.md`
   - 不足: 導入手順、対象ユーザー、主要ユースケース、機能マップ、FAQ

2. OSS 運営ドキュメント不在
   - 不足ファイル:
     - `CONTRIBUTING.md`
     - `SECURITY.md`
     - `CODE_OF_CONDUCT.md`
     - `CHANGELOG.md`
     - `.github/ISSUE_TEMPLATE/*`
     - `.github/pull_request_template.md`

3. リリース/配布ワークフロー不在
   - 根拠: `.github/workflows/` 配下が `ci.yml` のみ
   - 不足: タグ連動リリース、アーティファクト配布、チェックサム

4. バージョン情報が運用向きでない（`dev` 固定）
   - 根拠: `cmd/kra/main.go`, `Taskfile.yml`
   - 不足: `ldflags` で `version/commit/date` 注入

### P1: 公開直後に強く必要

1. AGENT バックログ未完了（5/8）
   - 根拠: `docs/backlog/README.md`, `docs/backlog/AGENT.md`
   - 未完:
     - `AGENT-050` tmux/zellij bridge
     - `AGENT-060` v2 inspection output
     - `AGENT-100` timeline/history

2. `agent` コマンドが experimental 限定
   - 根拠: `internal/cli/agent.go`, `internal/cli/agent_disabled.go`
   - 影響: stable 配布バイナリでは `agent` が使えない

3. JSON 契約の統一不足
   - 根拠: `docs/spec/concepts/output-contract.md`
   - 例外: `ws import jira` は独自 `--json` スキーマ
   - 影響: 外部自動化がコマンド横断で難しい

4. `ws select --multi` の JSON モード未実装
   - 根拠: `docs/spec/commands/ws/select-multi.md`

5. 非対話オートメーション適性が一部弱い
   - `ws create`: human 出力中心で機械可読出力不足
   - `repo discover`: 対話セレクタ前提で CI 組み込みが難しい

### P2: 中期で解消したい改善

1. provider 拡張性の実運用化（実質 GitHub 依存）
   - 根拠: `internal/repodiscovery/provider.go`

2. template サブコマンド機能の拡張余地
   - 現状は `template validate` 中心
   - テンプレート配布/管理 UX は未整備

3. ヘルプ整合性の細かな揺れ
   - `ws import jira` で `--board` 表記と非対応記述が混在

## 5. 強み（公開に向けたポジティブ要素）

1. 品質ゲートが明確で、実行可能な最小品質基準が定義済み
   - `gofmt`, UI color lint, `go vet`, `go test`
   - CI でも同等チェックが実施される

2. spec-first と backlog 連動の運用が確立されている

3. CLI の仕様・テスト密度が高く、内部品質は公開土台として強い

## 6. 推奨アクション（公開準備ロードマップ）

### Phase 1: 公開基盤（最優先）

1. README を公開向けに全面改訂
2. OSS 運営ドキュメント一式追加
3. リリースフロー（タグ -> バイナリ配布）を追加
4. バージョン注入方式を導入

### Phase 2: 自動化の使いやすさ

1. JSON 契約を主要コマンドで統一
2. `ws create` / `repo discover` の非対話モード強化
3. `ws select --multi` の JSON 対応

### Phase 3: 差別化機能の完成度向上

1. AGENT 系未完了チケットの実装
2. provider 拡張と plugin 方針検討
3. template lifecycle の強化

## 7. 公開可否の現時点判断

- 判定: **条件付きで公開見送り推奨**
- 条件:
  - Phase 1 の完了を公開条件とする
  - Phase 2 は公開直後マイルストーンで明記する

