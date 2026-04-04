package skeleton

type File struct {
	Path    string
	Imports []string
	Exports []Export
}

type Dir struct {
	Path  string
	Files []File
	Deps  []Dep // ファイル間の依存関係
}

// Dep は「target が source を import している」ことを表す
type Dep struct {
	Source string   // import されているファイル
	Users  []string // import しているファイル群
}

type Export struct {
	Kind      ExportKind
	Name      string
	Signature string // シグネチャ全体（"function verifyToken(token: string): Promise<JwtPayload>" 等）
}

type ExportKind int

const (
	ExportFunction ExportKind = iota
	ExportClass
	ExportInterface
	ExportType
	ExportVariable
)

func (k ExportKind) String() string {
	switch k {
	case ExportFunction:
		return "function"
	case ExportClass:
		return "class"
	case ExportInterface:
		return "interface"
	case ExportType:
		return "type"
	case ExportVariable:
		return "const"
	default:
		return "?"
	}
}
