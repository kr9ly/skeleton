package skeleton

// ModuleGraph はビルドシステムが定義するモジュール間の依存グラフ
type ModuleGraph struct {
	Root    string   // ルートディレクトリ
	System  string   // "gradle" 等、検出されたビルドシステム名
	Modules []Module // モジュール一覧（名前順）
}

type Module struct {
	Name string      // モジュール名（Gradle なら ":core:model" 等）
	Dir  string      // ルートからの相対ディレクトリ
	Deps []ModuleDep // このモジュールが依存するモジュール
}

type ModuleDep struct {
	Target string // 依存先モジュール名
	Kind   string // 依存の種類（Gradle なら "implementation", "api" 等）
}
