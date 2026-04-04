# skeleton: コードとドキュメントの骨格ビューア

> Read の前段として使う軽量ツール。
> ファイルの全文ではなく「骨格」だけを返すことで、
> トークン消費を抑えつつコードベースの構造を素早く把握する。

## 動機

AIエージェントがコードを読むとき、最初にやるのは構造の把握。
import、export、シグネチャ、型定義を見て当たりをつけ、
必要な箇所だけ Read で深掘りする。

しかし今の Read は全文を返すしかない。500行のファイルから
実際に必要な情報は20行程度。残りの480行はトークンの無駄遣い。

skeleton は「まず骨格を見て、必要なら Read」という
2段階ワークフローを実現する。

## 設計方針

- **インデックス不要。その場でパースして即返す**
  - codegraph のような事前構築・永続化は行わない
  - tree-sitter でパースし、宣言ノードだけ抽出して返す
- **言語は Go**。シングルバイナリ配布
- **MCPサーバーとして動作**（stdio transport）
- 対象はコードファイルとMarkdownファイル。出力フォーマットは統一

## MCPツール

### `skeleton`

ファイルまたはディレクトリの骨格を返す。

#### ファイル入力（コード）

```
入力: { path: "src/auth/jwt.ts" }
出力:
  imports:
    - ./config/env
    - jsonwebtoken
  exports:
    - function verifyToken(token: string): Promise<JwtPayload>
    - function signToken(payload: Record<string, unknown>, expiresIn?: string): string
    - interface JwtPayload { sub: string; role: Role; iat: number }
    - type Role = "admin" | "user"
```

ボディは一切含まない。関数ならシグネチャだけ、型なら定義だけ。

#### ファイル入力（Markdown）

```
入力: { path: "docs/architecture.md" }
出力:
  headings:
    - # Architecture Overview
      - ## Backend
        - ### API Layer
        - ### Database
      - ## Frontend
        - ### Component Structure
  links:
    - ./api-design.md
    - ./database-schema.md
    - ../README.md
```

見出しツリー + 内部リンク。本文は含まない。

#### ディレクトリ入力

```
入力: { path: "src/services/" }
出力:
  files:
    - auth.ts
      exports: [function login, function logout, class AuthService]
      imports_from: [../config/env, ../db/users]
    - payment.ts
      exports: [function charge, function refund]
      imports_from: [../config/env, ../db/orders, ./auth]
    - notification.ts
      exports: [function sendEmail, function sendPush]
      imports_from: [../config/env, ./auth]
  dependency_map:
    auth.ts <- payment.ts, notification.ts
    ../config/env <- auth.ts, payment.ts, notification.ts
    ../db/users <- auth.ts
    ../db/orders <- payment.ts
```

各ファイルの export 一覧 + ファイル間の import 関係。
ディレクトリ内の依存構造が一目でわかる。

### パラメータ

| パラメータ | 型 | デフォルト | 説明 |
|-----------|-----|----------|------|
| `path` | string | 必須 | ファイルまたはディレクトリのパス |
| `depth` | number | 1 | ディレクトリの場合の探索深度 |
| `lang` | string | 自動検出 | 言語を明示指定（拡張子で判定できない場合） |

## 実装

### パーサー

tree-sitter を使用。言語ごとの `.scm` クエリファイルで宣言ノードを抽出。

コードファイル:
- `@import.statement`, `@import.module` — import文
- `@export.declaration` — export宣言
- `@function.declaration`, `@function.name`, `@function.params`, `@function.return_type`
- `@class.declaration`, `@class.name`
- `@type.declaration`, `@type.name`, `@type.definition`
- `@interface.declaration`, `@interface.name`, `@interface.body`

Markdownファイル:
- `@heading`, `@heading.level`, `@heading.content` — 見出し
- `@link.destination` — リンク先パス（内部リンクのみ抽出）

### シグネチャ抽出

関数・メソッドの場合、ボディを除外してシグネチャだけを文字列化する。

```
// 入力（AST）
function verifyToken(token: string): Promise<JwtPayload> {
  const decoded = jwt.verify(token, SECRET);
  return { sub: decoded.sub, role: decoded.role, iat: decoded.iat };
}

// 出力
function verifyToken(token: string): Promise<JwtPayload>
```

型定義・インターフェースは浅い構造（フィールド名と型）を含める。
ネストした型は省略して `...` で示す。

### ディレクトリモード

1. 指定ディレクトリを depth まで走査
2. 各ファイルを個別にパースして export を収集
3. import 文を解決してファイル間の依存マップを構築
4. 結果をまとめて返す

import 解決は簡易的でよい:
- 相対パス → 拡張子探索（`.ts`, `.tsx`, `.js`, `/index.ts`）
- 非相対パス → `external:` として表示
- 解決できないものはそのまま表示

## 対応言語（初期）

| 言語 | 拡張子 | 備考 |
|------|--------|------|
| TypeScript / JavaScript | `.ts`, `.tsx`, `.js`, `.jsx` | export default 含む |
| Go | `.go` | 大文字始まり = export |
| Python | `.py` | `def`, `class`, `__all__` |
| Markdown | `.md` | 見出し + リンク |

追加は `.scm` クエリファイルの追加のみ。

## codegraph との関係

skeleton と codegraph は補完的な関係。

| | skeleton | codegraph |
|--|----------|-----------|
| 目的 | その場で骨格を見る（知覚） | 全体構造を分析する（分析） |
| インデックス | 不要 | 事前構築が必要 |
| 永続化 | なし | SQLite |
| 典型的な問い | 「このファイルの公開APIは？」 | 「最も重要なファイルは？」 |
| レイテンシ | ファイル: <100ms | 初回: 数秒〜数十秒 |

skeleton で骨格を掴み、codegraph で俯瞰する。
Read は skeleton で当たりをつけた後の精読用。

## やらないこと

- グラフメトリクス計算（PageRank等は codegraph の責務）
- Git履歴分析
- インデックスの永続化
- LLMドキュメント生成
- セマンティック検索
