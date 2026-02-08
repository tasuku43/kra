---
title: "Legacy SQLite Recovery"
status: implemented
---

# Legacy SQLite Recovery

## 対象

開発中の旧バージョンで `state.db` を生成済みの環境を、Filesystem-first運用へ切り替える手順。

## 方針

- 真実は `GIONX_ROOT` 配下のディレクトリと `.gionx.meta.json`。
- 旧 `state.db` は補助インデックス扱い。消してもワークスペース本体は消えない。
- まずバックアップを作成し、段階的に切り替える。

## 手順

1. 対象 root を確定する
```sh
gionx context current
```

2. 旧データをバックアップする
```sh
mkdir -p .gionx-recovery-backup
cp -a workspaces archive .gionx-recovery-backup/
```

3. 旧 `state.db` を退避する（存在する場合）
```sh
STATE_DB="$(find "${XDG_DATA_HOME:-$HOME/.local/share}/gionx/roots" -name state.db 2>/dev/null | head -n 1)"
if [ -n "$STATE_DB" ]; then
  mkdir -p .gionx-recovery-backup/legacy-index
  cp -a "$STATE_DB" .gionx-recovery-backup/legacy-index/
fi
```

4. `registry.json` の旧 `state_db_path` は放置でよい
- 新しい実装は `root_path` / timestamps を正準に扱う。
- コマンド実行時に registry が更新される。

5. 動作確認
```sh
gionx ws list
gionx ws select --act go
```

6. 問題なければ旧 `state.db` を削除（任意）
```sh
if [ -n "$STATE_DB" ]; then
  rm -f "$STATE_DB"
fi
```

## トラブル時

- `registry.json` 破損エラーが出る場合:
  - `XDG_DATA_HOME/gionx/registry.json` をバックアップして削除し、再実行。
- `ws --act reopen` が失敗する場合:
  - 対象 `archive/<id>/.gionx.meta.json` の `repos_restore` を確認。
  - bare repo が不足している場合は `gionx repo add` で再登録してから再実行。
