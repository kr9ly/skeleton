package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/kr9ly/skeleton/internal/edit"
	"github.com/kr9ly/skeleton/internal/extractor"
	"github.com/kr9ly/skeleton/internal/lang"
	"github.com/kr9ly/skeleton/internal/render"
	"github.com/kr9ly/skeleton/internal/scanner"
	"github.com/kr9ly/skeleton/internal/selector"
)

// flexInt は JSON の数値・文字列どちらからでもアンマーシャルできる int 型。
// MCP クライアントによっては number フィールドを文字列で送ることがある。
type flexInt int

func (f *flexInt) UnmarshalJSON(data []byte) error {
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		*f = flexInt(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		n, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("cannot parse %q as integer", s)
		}
		*f = flexInt(n)
		return nil
	}
	return fmt.Errorf("cannot unmarshal %s as integer", string(data))
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema inputSchema `json:"inputSchema"`
}

type inputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]property `json:"properties"`
	Required   []string            `json:"required"`
}

type property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Default     interface{} `json:"default,omitempty"`
	Enum        []string    `json:"enum,omitempty"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type callResult struct {
	Content []contentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

func Run() error {
	reader := bufio.NewReader(os.Stdin)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		// Notifications (no id) — just acknowledge
		if req.ID == nil {
			continue
		}

		resp := handle(req)
		out, _ := json.Marshal(resp)
		fmt.Fprintf(os.Stdout, "%s\n", out)
	}
}

func handle(req jsonRPCRequest) jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{},
				},
				"serverInfo": map[string]interface{}{
					"name":    "skeleton",
					"version": "0.1.0",
				},
			},
		}

	case "tools/list":
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"tools": getTools(),
			},
		}

	case "tools/call":
		return handleToolCall(req)

	default:
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32601, Message: "method not found: " + req.Method},
		}
	}
}

func getTools() []tool {
	return []tool{
		{
			Name:        "skeleton",
			Description: "ファイルまたはディレクトリのコード骨格（import, export, シグネチャ）を返す。Read の前段として使い、構造を素早く把握してトークンを節約する。未知のファイルを読む前・ディレクトリの関数/型を把握したいときにまず呼ぶ。Explore エージェントより軽量。",
			InputSchema: inputSchema{
				Type: "object",
				Properties: map[string]property{
					"path": {
						Type:        "string",
						Description: "ファイルまたはディレクトリの絶対パス",
					},
					"depth": {
						Type:        "number",
						Description: "ディレクトリ探索の深度",
						Default:     1,
					},
					"no_test": {
						Type:        "boolean",
						Description: "テストファイルを除外する",
						Default:     false,
					},
					"filter": {
						Type:        "string",
						Description: "ファイル名の glob パターンでフィルタする（例: \"*.kt\", \"*.ts\"）。ディレクトリモードのみ有効",
					},
				},
				Required: []string{"path"},
			},
		},
		{
			Name:        "skeleton_edit_insert",
			Description: "ASTノードセレクタで位置を指定してコードを挿入する。セレクタは skeleton ツールの出力から構成できる（例: function:getUser, class:AuthService, last:import）。",
			InputSchema: inputSchema{
				Type: "object",
				Properties: map[string]property{
					"path": {
						Type:        "string",
						Description: "対象ファイルの絶対パス",
					},
					"code": {
						Type:        "string",
						Description: "挿入するコード",
					},
					"position": {
						Type:        "string",
						Description: "挿入位置",
						Enum:        []string{"before", "after", "into", "into_head"},
					},
					"selector": {
						Type:        "string",
						Description: "ASTノードセレクタ（例: function:getUser, class:AuthService > method:findById, last:import）",
					},
				},
				Required: []string{"path", "code", "position", "selector"},
			},
		},
		{
			Name:        "skeleton_edit_remove",
			Description: "ASTノードセレクタで指定したノードを削除する。",
			InputSchema: inputSchema{
				Type: "object",
				Properties: map[string]property{
					"path": {
						Type:        "string",
						Description: "対象ファイルの絶対パス",
					},
					"selector": {
						Type:        "string",
						Description: "削除するASTノードのセレクタ",
					},
					"dry_run": {
						Type:        "boolean",
						Description: "true の場合、ファイルを変更せず結果を返す",
						Default:     false,
					},
				},
				Required: []string{"path", "selector"},
			},
		},
	}
}

func handleToolCall(req jsonRPCRequest) jsonRPCResponse {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32602, Message: "invalid params"},
		}
	}

	var result callResult
	switch params.Name {
	case "skeleton":
		result = toolSkeleton(params.Arguments)
	case "skeleton_edit_insert":
		result = toolEditInsert(params.Arguments)
	case "skeleton_edit_remove":
		result = toolEditRemove(params.Arguments)
	default:
		result = errResult("unknown tool: " + params.Name)
	}

	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func toolSkeleton(raw json.RawMessage) callResult {
	var args struct {
		Path   string  `json:"path"`
		Depth  flexInt `json:"depth"`
		NoTest bool    `json:"no_test"`
		Filter string  `json:"filter"`
	}
	args.Depth = 1
	if err := json.Unmarshal(raw, &args); err != nil {
		return errResult("invalid arguments: " + err.Error())
	}

	info, err := os.Stat(args.Path)
	if err != nil {
		return errResult(err.Error())
	}

	if info.IsDir() {
		opts := scanner.Options{Depth: int(args.Depth), NoTest: args.NoTest, Filter: args.Filter}
		dir, err := scanner.ScanDir(args.Path, opts)
		if err != nil {
			return errResult(err.Error())
		}
		return textResult(render.TextDir(dir))
	}

	src, err := os.ReadFile(args.Path)
	if err != nil {
		return errResult(err.Error())
	}

	language := lang.Detect(args.Path)
	if language == lang.Unknown {
		return errResult("unsupported file type: " + args.Path)
	}

	ext := newExtractor(language)
	if ext == nil {
		return errResult("unsupported language: " + args.Path)
	}

	file, err := ext.Extract(src)
	if err != nil {
		return errResult("parse error: " + err.Error())
	}
	file.Path = args.Path
	return textResult(render.Text(file))
}

func toolEditInsert(raw json.RawMessage) callResult {
	var args struct {
		Path     string `json:"path"`
		Code     string `json:"code"`
		Position string `json:"position"`
		Selector string `json:"selector"`
		DryRun   bool   `json:"dry_run"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return errResult("invalid arguments: " + err.Error())
	}

	src, err := os.ReadFile(args.Path)
	if err != nil {
		return errResult(err.Error())
	}

	sel, err := selector.Parse(args.Selector)
	if err != nil {
		return errResult("selector error: " + err.Error())
	}

	var result []byte
	switch args.Position {
	case "before":
		result, err = edit.InsertBefore(src, args.Path, sel, args.Code)
	case "after":
		result, err = edit.InsertAfter(src, args.Path, sel, args.Code)
	case "into":
		result, err = edit.InsertInto(src, args.Path, sel, args.Code, false)
	case "into_head":
		result, err = edit.InsertInto(src, args.Path, sel, args.Code, true)
	default:
		return errResult("invalid position: " + args.Position)
	}

	if err != nil {
		return errResult("edit error: " + err.Error())
	}

	if args.DryRun {
		return textResult(string(result))
	}

	if err := os.WriteFile(args.Path, result, 0644); err != nil {
		return errResult("write error: " + err.Error())
	}
	return textResult("inserted into " + args.Path)
}

func toolEditRemove(raw json.RawMessage) callResult {
	var args struct {
		Path     string `json:"path"`
		Selector string `json:"selector"`
		DryRun   bool   `json:"dry_run"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return errResult("invalid arguments: " + err.Error())
	}

	src, err := os.ReadFile(args.Path)
	if err != nil {
		return errResult(err.Error())
	}

	sel, err := selector.Parse(args.Selector)
	if err != nil {
		return errResult("selector error: " + err.Error())
	}

	result, err := edit.Remove(src, args.Path, sel)
	if err != nil {
		return errResult("edit error: " + err.Error())
	}

	if args.DryRun {
		return textResult(string(result))
	}

	if err := os.WriteFile(args.Path, result, 0644); err != nil {
		return errResult("write error: " + err.Error())
	}
	return textResult("removed from " + args.Path)
}

func newExtractor(language lang.Language) extractor.Extractor {
	switch language {
	case lang.TypeScript:
		return extractor.NewTypeScript()
	case lang.Python:
		return extractor.NewPython()
	case lang.Go:
		return extractor.NewGo()
	case lang.Markdown:
		return extractor.NewMarkdown()
	case lang.Kotlin:
		return extractor.NewKotlin()
	default:
		return nil
	}
}

func textResult(text string) callResult {
	return callResult{
		Content: []contentItem{{Type: "text", Text: text}},
	}
}

func errResult(msg string) callResult {
	return callResult{
		Content: []contentItem{{Type: "text", Text: msg}},
		IsError: true,
	}
}
