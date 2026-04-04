package skeleton

type File struct {
	Path    string
	Imports []string
	Exports []Export
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
