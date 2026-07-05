package skeleton

// LibsReport はビルドシステムが宣言する外部ライブラリ依存の一覧。
// 宣言ベース（直接依存のみ）であり、推移的依存や解決後のバージョンは扱わない。
type LibsReport struct {
	Root   string
	System string     // "gradle" 等、検出されたビルドシステム名
	Scopes []LibScope // モジュールシステムがあればモジュール単位、なければ単一スコープ
}

type LibScope struct {
	Name string // モジュール名（単一スコープなら ":"）
	Libs []Lib
}

type Lib struct {
	Coordinate string // "io.ktor:ktor-client-core" 等。解決できない場合は宣言表記のまま
	Version    string // 宣言バージョン（不明なら空）
	Kind       string // 依存の種類（Gradle なら "implementation", "testImplementation" 等）
}
