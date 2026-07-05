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

// Extract は root に一致する Provider を検出してモジュールグラフを抽出する
func Extract(root string) (*skeleton.ModuleGraph, error) {
	for _, p := range providers {
		if p.Detect(root) {
			return p.Extract(root)
		}
	}
	var names []string
	for _, p := range providers {
		names = append(names, p.Name())
	}
	return nil, fmt.Errorf("no supported build system found in %s (supported: %s)", root, strings.Join(names, ", "))
}
