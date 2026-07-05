// Package moduledep はビルドシステムごとのモジュール依存グラフ抽出を提供する。
// Provider を実装して providers に追加すると新しいビルドシステムに対応できる。
package moduledep

import (
	"fmt"
	"strings"

	"github.com/kr9ly/skeleton/skeleton"
)

type Provider interface {
	Name() string
	// Detect は root がこのビルドシステムのプロジェクトかを判定する
	Detect(root string) bool
	Extract(root string) (*skeleton.ModuleGraph, error)
}

var providers = []Provider{
	&gradleProvider{},
}

// LibsProvider は外部ライブラリ依存の一覧抽出に対応した Provider
type LibsProvider interface {
	Provider
	ExtractLibs(root string) (*skeleton.LibsReport, error)
}

// Extract は root に一致する Provider を検出してモジュールグラフを抽出する
func Extract(root string) (*skeleton.ModuleGraph, error) {
	for _, p := range providers {
		if p.Detect(root) {
			return p.Extract(root)
		}
	}
	return nil, errNoBuildSystem(root)
}

// ExtractLibs は root に一致する Provider を検出して外部ライブラリ一覧を抽出する
func ExtractLibs(root string) (*skeleton.LibsReport, error) {
	for _, p := range providers {
		if p.Detect(root) {
			lp, ok := p.(LibsProvider)
			if !ok {
				return nil, fmt.Errorf("build system %s does not support library extraction", p.Name())
			}
			return lp.ExtractLibs(root)
		}
	}
	return nil, errNoBuildSystem(root)
}

func errNoBuildSystem(root string) error {
	var names []string
	for _, p := range providers {
		names = append(names, p.Name())
	}
	return fmt.Errorf("no supported build system found in %s (supported: %s)", root, strings.Join(names, ", "))
}
