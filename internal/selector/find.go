package selector

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// Match は見つかったノードとその位置情報。
type Match struct {
	Node *sitter.Node
}

// Find はセレクタに一致するノードを探す。
func Find(sel Selector, root *sitter.Node, src []byte) (*Match, bool) {
	node := root
	for i, part := range sel.Parts {
		searchIn := node
		// 2段目以降は body の中を探す
		if i > 0 {
			body := FindBody(node)
			if body != nil {
				searchIn = body
			}
		}
		found := findPart(part, searchIn, src)
		if found == nil {
			return nil, false
		}
		node = found
	}
	return &Match{Node: node}, true
}

func findPart(part Part, parent *sitter.Node, src []byte) *sitter.Node {
	candidates := collectCandidates(part.Kind, parent, src)
	if len(candidates) == 0 {
		return nil
	}

	switch part.Pos {
	case PosFirst:
		return candidates[0]
	case PosLast:
		return candidates[len(candidates)-1]
	case PosNth:
		if part.N < 0 || part.N >= len(candidates) {
			return nil
		}
		return candidates[part.N]
	case PosName:
		for _, c := range candidates {
			if nodeName(c, src) == part.Name || part.Name == "*" {
				return c
			}
		}
		return nil
	}
	return nil
}

func collectCandidates(kind string, parent *sitter.Node, src []byte) []*sitter.Node {
	var results []*sitter.Node
	iterateChildren(parent, src, func(child *sitter.Node) {
		if matchesKind(kind, child, src) {
			results = append(results, child)
		}
	})
	return results
}

// iterateChildren は直接の子と、export_statement / decorated_definition の中身を走査する。
func iterateChildren(parent *sitter.Node, src []byte, fn func(*sitter.Node)) {
	for i := 0; i < int(parent.NamedChildCount()); i++ {
		child := parent.NamedChild(i)

		switch child.Type() {
		case "export_statement":
			// export の中の declaration を直接扱う
			if decl := child.ChildByFieldName("declaration"); decl != nil {
				fn(decl)
			}
			// export 文自体も import:* のような用途で
			fn(child)
		case "decorated_definition":
			if def := child.ChildByFieldName("definition"); def != nil {
				fn(def)
			}
		default:
			fn(child)
		}
	}
}

func matchesKind(kind string, node *sitter.Node, src []byte) bool {
	nodeType := node.Type()
	switch kind {
	case "function":
		return nodeType == "function_declaration" ||
			nodeType == "function_definition" ||
			nodeType == "arrow_function"
	case "class":
		return nodeType == "class_declaration" ||
			nodeType == "class_definition"
	case "interface":
		return nodeType == "interface_declaration"
	case "type":
		return nodeType == "type_alias_declaration" ||
			nodeType == "type_alias_statement"
	case "import":
		return nodeType == "import_statement" ||
			nodeType == "import_from_statement"
	case "export":
		return nodeType == "export_statement"
	case "method":
		return nodeType == "method_definition" ||
			nodeType == "method_signature"
	case "field":
		return nodeType == "public_field_definition" ||
			nodeType == "property_signature"
	default:
		return false
	}
}

func nodeName(node *sitter.Node, src []byte) string {
	// name フィールドを持つノード
	if name := node.ChildByFieldName("name"); name != nil {
		return name.Content(src)
	}
	// import の場合は source を名前として扱う
	if node.Type() == "import_statement" || node.Type() == "import_from_statement" {
		if s := node.ChildByFieldName("source"); s != nil {
			return unquote(s.Content(src))
		}
		if s := node.ChildByFieldName("module_name"); s != nil {
			return s.Content(src)
		}
	}
	return ""
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// FindBody はノードの body（class_body, statement_block 等）を返す。
func FindBody(node *sitter.Node) *sitter.Node {
	body := node.ChildByFieldName("body")
	if body != nil {
		return body
	}
	// class_body, interface_body 等の名前で探す
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		t := child.Type()
		if strings.HasSuffix(t, "_body") || t == "statement_block" || t == "block" || t == "object_type" {
			return child
		}
	}
	return nil
}
