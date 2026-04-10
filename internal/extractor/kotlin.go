package extractor

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/kotlin"

	"github.com/kr9ly/skeleton/skeleton"
)

type KotlinExtractor struct{}

func NewKotlin() *KotlinExtractor {
	return &KotlinExtractor{}
}

func (e *KotlinExtractor) Extract(src []byte) (*skeleton.File, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(kotlin.GetLanguage())

	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	root := tree.RootNode()
	file := &skeleton.File{}

	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		switch child.Type() {
		case "package_header":
			// package com.example → used as module identifier, skip for imports
		case "import_list":
			for j := 0; j < int(child.NamedChildCount()); j++ {
				imp := child.NamedChild(j)
				if imp.Type() == "import_header" {
					if path := kotlinImportPath(imp, src); path != "" {
						file.Imports = append(file.Imports, path)
					}
				}
			}
		case "type_alias":
			if exp := kotlinExtractTypeAlias(child, src); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}
		case "property_declaration":
			if kotlinIsPrivate(child, src) {
				continue
			}
			if exp := kotlinExtractProperty(child, src); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}
		case "function_declaration":
			if kotlinIsPrivate(child, src) {
				continue
			}
			if exp := kotlinExtractFunction(child, src); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}
		case "class_declaration":
			if kotlinIsPrivate(child, src) {
				continue
			}
			if exp := kotlinExtractClass(child, src); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}
		case "object_declaration":
			if kotlinIsPrivate(child, src) {
				continue
			}
			if exp := kotlinExtractObject(child, src); exp != nil {
				file.Exports = append(file.Exports, *exp)
			}
		}
	}

	return file, nil
}

// kotlinImportPath extracts the dotted path from an import_header node.
func kotlinImportPath(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" {
			return content(child, src)
		}
	}
	return ""
}

// kotlinIsPrivate returns true if the node has a modifiers child containing
// a visibility_modifier of private, internal, or protected.
func kotlinIsPrivate(node *sitter.Node, src []byte) bool {
	modifiers := findChildByType(node, "modifiers")
	if modifiers == nil {
		return false
	}
	for i := 0; i < int(modifiers.ChildCount()); i++ {
		child := modifiers.Child(i)
		if child.Type() == "visibility_modifier" {
			vis := strings.TrimSpace(content(child, src))
			switch vis {
			case "private", "internal", "protected":
				return true
			}
		}
	}
	return false
}

// kotlinExtractTypeAlias extracts a typealias declaration.
func kotlinExtractTypeAlias(node *sitter.Node, src []byte) *skeleton.Export {
	// type_alias: typealias <type_identifier> = <type>
	name := ""
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_identifier" {
			name = content(child, src)
			break
		}
	}
	if name == "" {
		return nil
	}
	sig := "typealias " + strings.TrimSpace(content(node, src))
	// Remove "typealias " prefix duplication — content already includes it
	if strings.HasPrefix(sig, "typealias typealias") {
		sig = sig[len("typealias "):]
	}
	return &skeleton.Export{
		Kind:      skeleton.ExportType,
		Name:      name,
		Signature: strings.TrimSpace(content(node, src)),
	}
}

// kotlinExtractProperty extracts a top-level val/var property.
func kotlinExtractProperty(node *sitter.Node, src []byte) *skeleton.Export {
	varDecl := findChildByType(node, "variable_declaration")
	if varDecl == nil {
		return nil
	}
	name := kotlinSimpleIdentifier(varDecl, src)
	if name == "" {
		return nil
	}

	// binding_pattern_kind contains val or var
	kind := "val"
	bpk := findChildByType(node, "binding_pattern_kind")
	if bpk != nil {
		kind = strings.TrimSpace(content(bpk, src))
	}

	// type annotation in variable_declaration after ':'
	typeSig := kotlinVariableType(varDecl, src)
	sig := kind + " " + name
	if typeSig != "" {
		sig += ": " + typeSig
	}
	return &skeleton.Export{
		Kind:      skeleton.ExportVariable,
		Name:      name,
		Signature: sig,
	}
}

// kotlinExtractFunction extracts a function declaration signature.
func kotlinExtractFunction(node *sitter.Node, src []byte) *skeleton.Export {
	name := kotlinFuncName(node, src)
	if name == "" {
		return nil
	}
	sig := kotlinFuncSignature(node, src)
	return &skeleton.Export{
		Kind:      skeleton.ExportFunction,
		Name:      name,
		Signature: sig,
	}
}

// kotlinExtractClass extracts class/data class/sealed class/enum class/interface.
func kotlinExtractClass(node *sitter.Node, src []byte) *skeleton.Export {
	name := ""
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_identifier" {
			name = content(child, src)
			break
		}
	}
	if name == "" {
		return nil
	}

	// Determine if it's interface, enum, data, sealed, or plain class
	isInterface := false
	isEnum := false
	prefix := ""
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "interface":
			isInterface = true
		case "enum":
			isEnum = true
		case "modifiers":
			// data, sealed, abstract, etc.
			modText := strings.TrimSpace(content(child, src))
			prefix = modText + " "
		}
	}

	var kind skeleton.ExportKind
	var classKeyword string
	if isInterface {
		kind = skeleton.ExportInterface
		classKeyword = "interface"
	} else {
		kind = skeleton.ExportClass
		if isEnum {
			classKeyword = "enum class"
			prefix = "" // enum keyword is already in classKeyword
		} else {
			classKeyword = "class"
		}
	}

	// Build signature: modifiers + keyword + name + primary_constructor (params)
	sig := prefix + classKeyword + " " + name
	pc := findChildByType(node, "primary_constructor")
	if pc != nil {
		sig += kotlinPrimaryConstructorSig(pc, src)
	}

	// Extract members from class_body or enum_class_body
	var members []skeleton.Member
	body := findChildByType(node, "class_body")
	if body == nil {
		body = findChildByType(node, "enum_class_body")
	}
	if body != nil {
		members = kotlinExtractClassMembers(body, src)
	}
	// Also extract primary constructor params as fields (val/var params)
	if pc != nil {
		members = append(kotlinExtractConstructorFields(pc, src), members...)
	}

	return &skeleton.Export{
		Kind:      kind,
		Name:      name,
		Signature: strings.TrimSpace(sig),
		Members:   members,
	}
}

// kotlinExtractObject extracts an object declaration (singleton).
func kotlinExtractObject(node *sitter.Node, src []byte) *skeleton.Export {
	name := ""
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_identifier" {
			name = content(child, src)
			break
		}
	}
	if name == "" {
		return nil
	}

	var members []skeleton.Member
	body := findChildByType(node, "class_body")
	if body != nil {
		members = kotlinExtractClassMembers(body, src)
	}

	return &skeleton.Export{
		Kind:      skeleton.ExportClass,
		Name:      name,
		Signature: "object " + name,
		Members:   members,
	}
}

// kotlinExtractClassMembers extracts public members from a class_body or enum_class_body.
func kotlinExtractClassMembers(body *sitter.Node, src []byte) []skeleton.Member {
	var members []skeleton.Member
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		switch child.Type() {
		case "enum_entry":
			name := kotlinSimpleIdentifier(child, src)
			if name != "" {
				members = append(members, skeleton.Member{
					Kind:      skeleton.MemberField,
					Name:      name,
					Signature: name,
				})
			}
		case "property_declaration":
			if kotlinIsPrivate(child, src) {
				continue
			}
			varDecl := findChildByType(child, "variable_declaration")
			if varDecl == nil {
				continue
			}
			name := kotlinSimpleIdentifier(varDecl, src)
			if name == "" {
				continue
			}
			bpk := findChildByType(child, "binding_pattern_kind")
			kind := "val"
			if bpk != nil {
				kind = strings.TrimSpace(content(bpk, src))
			}
			typeSig := kotlinVariableType(varDecl, src)
			sig := kind + " " + name
			if typeSig != "" {
				sig += ": " + typeSig
			}
			members = append(members, skeleton.Member{
				Kind:      skeleton.MemberField,
				Name:      name,
				Signature: sig,
			})
		case "function_declaration":
			if kotlinIsPrivate(child, src) {
				continue
			}
			name := kotlinFuncName(child, src)
			if name == "" {
				continue
			}
			sig := kotlinFuncSignature(child, src)
			members = append(members, skeleton.Member{
				Kind:      skeleton.MemberMethod,
				Name:      name,
				Signature: sig,
			})
		}
	}
	return members
}

// kotlinExtractConstructorFields extracts val/var parameters from a primary_constructor as fields.
func kotlinExtractConstructorFields(pc *sitter.Node, src []byte) []skeleton.Member {
	var members []skeleton.Member
	for i := 0; i < int(pc.ChildCount()); i++ {
		child := pc.Child(i)
		if child.Type() != "class_parameter" {
			continue
		}
		// class_parameter has val/var only if binding_pattern_kind is present
		bpk := findChildByType(child, "binding_pattern_kind")
		if bpk == nil {
			continue // plain constructor param, not a property
		}
		if kotlinIsPrivate(child, src) {
			continue
		}
		name := kotlinSimpleIdentifier(child, src)
		if name == "" {
			continue
		}
		kind := strings.TrimSpace(content(bpk, src))
		typeSig := kotlinVariableType(child, src)
		sig := kind + " " + name
		if typeSig != "" {
			sig += ": " + typeSig
		}
		members = append(members, skeleton.Member{
			Kind:      skeleton.MemberField,
			Name:      name,
			Signature: sig,
		})
	}
	return members
}

// kotlinPrimaryConstructorSig returns the parameter list string for the signature.
func kotlinPrimaryConstructorSig(pc *sitter.Node, src []byte) string {
	// Just grab from '(' to ')'
	start := pc.StartByte()
	end := pc.EndByte()
	return strings.TrimSpace(string(src[start:end]))
}

// kotlinFuncName returns the simple_identifier name of a function_declaration.
func kotlinFuncName(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "simple_identifier" {
			return content(child, src)
		}
	}
	return ""
}

// kotlinFuncSignature returns the signature without the body.
func kotlinFuncSignature(node *sitter.Node, src []byte) string {
	body := findChildByType(node, "function_body")
	if body == nil {
		return strings.TrimSpace(content(node, src))
	}
	sig := strings.TrimSpace(string(src[node.StartByte():body.StartByte()]))
	return sig
}

// kotlinSimpleIdentifier finds the first simple_identifier child of a node.
func kotlinSimpleIdentifier(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "simple_identifier" {
			return content(child, src)
		}
	}
	return ""
}

// kotlinVariableType extracts the type annotation from variable_declaration or class_parameter.
// The type follows ':' in the AST as a user_type or nullable_type node.
func kotlinVariableType(node *sitter.Node, src []byte) string {
	colonSeen := false
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == ":" {
			colonSeen = true
			continue
		}
		if colonSeen {
			// Next named node after ':' is the type
			t := strings.TrimSpace(content(child, src))
			if t != "" {
				return t
			}
		}
	}
	return ""
}
