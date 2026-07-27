# skeleton

コードとドキュメントの骨格ビューア + AST エディタ。

ファイルの全文ではなく「骨格」（import / export / シグネチャ / 見出し）だけを返すことで、
トークン消費を抑えつつコードベースの構造を素早く把握する。
AI エージェントの「まず骨格を見て、必要なら Read」という 2 段階ワークフローのためのツール。
各シンボルには定義の行範囲（` :12-45`）が付くので、そのまま行指定の部分 Read に直結できる。

- インデックス不要。tree-sitter でその場でパースして即返す
- Go 製シングルバイナリ
- CLI と MCP サーバー（stdio）の両対応

## 対応言語

| 言語 | 拡張子 |
|------|--------|
| TypeScript / JavaScript | `.ts` `.tsx` `.mts` `.cts` `.js` `.jsx` `.mjs` `.cjs` |
| Python | `.py` |
| Go | `.go` |
| Kotlin | `.kt` `.kts` |
| Java | `.java` |
| C | `.c` `.h` |
| C++ | `.cpp` `.hpp` `.cc` `.hh` `.cxx` `.hxx` |
| CUDA | `.cu` `.cuh` |
| Zig | `.zig` |
| GLSL | `.glsl` `.vert` `.frag` `.comp` `.geom` `.tesc` `.tese` |
| Markdown | `.md`（見出しツリーを抽出） |

## インストール

### リリースバイナリ（推奨）

[Releases](https://github.com/kr9ly/skeleton/releases) から自分のプラットフォームのバイナリをダウンロードする。

| プラットフォーム | アセット |
|------------------|----------|
| macOS (Apple Silicon) | `skeleton-darwin-arm64` |
| macOS (Intel) | `skeleton-darwin-amd64` |
| Linux (x86_64) | `skeleton-linux-amd64` |

```bash
# 例: macOS (Apple Silicon)
curl -L -o ~/.local/bin/skeleton \
  https://github.com/kr9ly/skeleton/releases/latest/download/skeleton-darwin-arm64
chmod +x ~/.local/bin/skeleton
```

`~/.local/bin` が PATH に含まれていることを確認すること。

> **macOS の注意**: バイナリは署名・公証されていない。
> `curl` や `gh release download` でのダウンロードなら quarantine 属性が付かずそのまま実行できるが、
> ブラウザでダウンロードした場合は Gatekeeper にブロックされるため、以下で解除する:
>
> ```bash
> xattr -d com.apple.quarantine ~/.local/bin/skeleton
> ```

### ソースからビルド

Go 1.22 以降と C コンパイラが必要（tree-sitter が CGO 依存）。

```bash
git clone https://github.com/kr9ly/skeleton.git
cd skeleton
make install   # ~/.local/bin/skeleton にインストール
```

## CLI の使い方

### 骨格表示

```bash
skeleton <file>                 # ファイルの骨格（import/export/シグネチャ）
skeleton <dir>                  # ディレクトリ直下の骨格一覧
skeleton -depth 2 <dir>         # 探索深度を指定
skeleton -no-test <dir>         # テストファイルを除外
skeleton -filter '*.kt' <dir>   # glob でファイルを絞り込み
```

### モジュール依存グラフ

マルチモジュールプロジェクトのモジュール間依存を可視化する。ビルドシステムは自動検出される（現在は Gradle に対応）。

```bash
skeleton modules [dir]          # モジュール一覧 + 依存 + 逆依存（dir 省略時はカレント）
```

出力例:

```
# /path/to/project (gradle, 5 modules)

:app
  -> :core:model (implementation)
  -> :core:ui (implementation)
:core:model
:core:ui
  -> :core:model (api)

## reverse deps
:core:model <- :app, :core:ui
```

Gradle 対応の詳細:

- `settings.gradle` / `settings.gradle.kts` の `include` 宣言からモジュールを列挙（`projectDir` 上書きにも対応）
- 各モジュールの `build.gradle(.kts)` から `project(":x")` 依存を抽出（Groovy / Kotlin DSL 両対応）
- typesafe project accessor（`implementation(projects.coreModel)`）に対応
- `apply from: rootProject.file(...)` 等の共有スクリプト経由の依存宣言も再帰的に追跡

### 外部ライブラリ一覧

プロジェクトが**宣言する**外部ライブラリ依存を、ライブラリ → 使用モジュールの逆引きで一覧する。
バージョンがモジュール間で割れている場合はバージョンごとに表示される。

```bash
skeleton libs [dir]             # 宣言ライブラリ一覧（dir 省略時はカレント）
```

出力例:

```
# /path/to/project (gradle, 42 libraries)

com.squareup.okhttp3:okhttp
  4.11.0 <- :legacy-lib
  4.12.0 <- :app, :feature:home
com.squareup.moshi:moshi-kotlin 1.15.0 <- :core:model
androidx.core:core-ktx 1.13.1 <- (91 modules)
```

- version catalog（`gradle/libs.versions.toml`）の accessor / bundle を座標に解決する
- モジュールシステムがない単一プロジェクトでも動く（使用元リストを省いた単純な一覧になる）
- **宣言ベース（直接依存のみ）**: 推移的依存・BOM 適用後の実バージョン・競合解決の結果が必要なら `gradle dependencies` を使うこと
- Groovy 変数など解決できない表記は宣言のまま出力する（例: `com.x:y $gsonVersion`）

### AST 編集

コードの挿入はコードを stdin から渡す。位置は AST ノードセレクタで指定する。

```bash
# 挿入: --before / --after / --into / --into-head のいずれか 1 つを指定
skeleton edit insert --after "last:import" src/app.ts <<< 'import { z } from "zod";'
skeleton edit insert --into "class:AuthService" src/auth.ts <<< '  logout(): void {}'

# 削除
skeleton edit remove "function:legacyHandler" src/app.ts

# dry-run: ファイルを変更せず結果を stdout に出す
skeleton edit insert --dry-run --after "last:import" src/app.ts <<< 'code'
skeleton edit remove --dry-run "function:foo" src/app.ts
```

### セレクタ構文

```
kind:name          — 名前で指定       function:getUser, class:AuthService
first:kind         — 最初のノード      first:import
last:kind          — 最後のノード      last:import
nth:kind:N         — N番目            nth:import:3
parent > child     — ネスト           class:Foo > method:bar
```

kind: `function` `class` `interface` `type` `import` `export` `method` `field`

## MCP サーバーとして使う

`skeleton mcp` で stdio transport の MCP サーバーとして起動する。

### Claude Code への登録

`~/.claude.json` の `mcpServers` に追加:

```json
"skeleton": {
  "type": "stdio",
  "command": "/Users/<you>/.local/bin/skeleton",
  "args": ["mcp"],
  "env": {}
}
```

登録は次回セッションから有効。

### MCP ツール

#### `skeleton` — 骨格表示

| パラメータ | 型 | 必須 | デフォルト | 説明 |
|-----------|-----|------|-----------|------|
| `path` | string | Yes | — | ファイルまたはディレクトリの絶対パス |
| `depth` | number | No | 1 | ディレクトリ探索の深度 |
| `no_test` | boolean | No | false | テストファイルを除外 |

#### `skeleton_modules` — モジュール依存グラフ

| パラメータ | 型 | 必須 | 説明 |
|-----------|-----|------|------|
| `path` | string | Yes | プロジェクトルートの絶対パス |

#### `skeleton_libs` — 外部ライブラリ一覧

| パラメータ | 型 | 必須 | 説明 |
|-----------|-----|------|------|
| `path` | string | Yes | プロジェクトルートの絶対パス |

#### `skeleton_edit_insert` — AST 位置指定でコード挿入

| パラメータ | 型 | 必須 | 説明 |
|-----------|-----|------|------|
| `path` | string | Yes | 対象ファイルの絶対パス |
| `code` | string | Yes | 挿入するコード |
| `position` | string | Yes | `before` / `after` / `into` / `into_head` |
| `selector` | string | Yes | AST ノードセレクタ |
| `dry_run` | boolean | No | true ならファイル変更せず結果を返す |

#### `skeleton_edit_remove` — AST ノード削除

| パラメータ | 型 | 必須 | 説明 |
|-----------|-----|------|------|
| `path` | string | Yes | 対象ファイルの絶対パス |
| `selector` | string | Yes | 削除するノードのセレクタ |
| `dry_run` | boolean | No | true ならファイル変更せず結果を返す |

## リリース手順（メンテナ向け)

タグを push すると GitHub Actions が macOS (arm64 / amd64) と Linux (amd64) のバイナリをビルドし、
checksums.txt とともに Release にアップロードする。

```bash
git tag v0.1.0
git push origin v0.1.0
```
