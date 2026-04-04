package selector

import (
	"fmt"
	"strconv"
	"strings"
)

// Selector はノードの位置を指定する。
// 例: "function:getUser", "last:import", "class:Foo > method:bar"
type Selector struct {
	Parts []Part
}

type Part struct {
	Kind string // function, class, interface, type, import, export, method, field
	Name string // 名前（"*" は全マッチ、空は位置指定）
	Pos  Pos    // first, last, nth
	N    int    // nth の場合のインデックス（0-based）
}

type Pos int

const (
	PosName Pos = iota // kind:name 形式
	PosFirst           // first:kind
	PosLast            // last:kind
	PosNth             // nth:kind:N
)

// Parse はセレクタ文字列をパースする。
func Parse(s string) (Selector, error) {
	segments := strings.Split(s, ">")
	var parts []Part
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		p, err := parsePart(seg)
		if err != nil {
			return Selector{}, err
		}
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		return Selector{}, fmt.Errorf("empty selector")
	}
	return Selector{Parts: parts}, nil
}

func parsePart(s string) (Part, error) {
	tokens := strings.SplitN(s, ":", -1)
	if len(tokens) < 2 {
		return Part{}, fmt.Errorf("invalid selector part: %q (expected kind:name)", s)
	}

	first := tokens[0]
	switch first {
	case "first":
		if len(tokens) < 2 {
			return Part{}, fmt.Errorf("invalid selector: %q", s)
		}
		return Part{Kind: tokens[1], Pos: PosFirst}, nil
	case "last":
		if len(tokens) < 2 {
			return Part{}, fmt.Errorf("invalid selector: %q", s)
		}
		return Part{Kind: tokens[1], Pos: PosLast}, nil
	case "nth":
		if len(tokens) < 3 {
			return Part{}, fmt.Errorf("invalid selector: %q (expected nth:kind:N)", s)
		}
		n, err := strconv.Atoi(tokens[2])
		if err != nil {
			return Part{}, fmt.Errorf("invalid nth index in %q: %w", s, err)
		}
		return Part{Kind: tokens[1], Pos: PosNth, N: n}, nil
	default:
		// kind:name 形式
		name := strings.Join(tokens[1:], ":")
		return Part{Kind: first, Name: name, Pos: PosName}, nil
	}
}
