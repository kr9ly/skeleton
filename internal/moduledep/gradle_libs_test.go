package moduledep

import (
	"reflect"
	"testing"

	"github.com/kr9ly/skeleton/skeleton"
)

func TestGradleExtractLibs(t *testing.T) {
	g := &gradleProvider{}
	report, err := g.ExtractLibs("testdata/gradle-sample")
	if err != nil {
		t.Fatal(err)
	}

	byName := make(map[string][]skeleton.Lib)
	var names []string
	for _, s := range report.Scopes {
		names = append(names, s.Name)
		byName[s.Name] = s.Libs
	}

	// 外部依存のないスコープ（:core:ui, :util, ルート）は含まれない
	wantNames := []string{":app", ":core:model", ":feature:home", ":legacy-lib"}
	if !reflect.DeepEqual(names, wantNames) {
		t.Fatalf("scopes = %v, want %v", names, wantNames)
	}

	// 文字列座標 / bundle 展開 / platform() ラッパー
	wantApp := []skeleton.Lib{
		{Coordinate: "androidx.compose:compose-bom", Version: "2024.05.00", Kind: "implementation"},
		{Coordinate: "com.squareup.okhttp3:okhttp", Version: "4.12.0", Kind: "implementation"},
		{Coordinate: "com.squareup.retrofit2:retrofit", Version: "2.9.0", Kind: "implementation"},
		{Coordinate: "io.ktor:ktor-client-core", Version: "2.3.0", Kind: "implementation"},
	}
	if got := byName[":app"]; !reflect.DeepEqual(got, wantApp) {
		t.Errorf(":app libs = %v, want %v", got, wantApp)
	}

	// apply from 経由 + catalog の version.ref 解決
	wantModel := []skeleton.Lib{
		{Coordinate: "com.squareup.moshi:moshi-kotlin", Version: "1.15.0", Kind: "implementation"},
	}
	if got := byName[":core:model"]; !reflect.DeepEqual(got, wantModel) {
		t.Errorf(":core:model libs = %v, want %v", got, wantModel)
	}

	// Groovy 変数入りバージョンはそのまま / シングルクォート TOML の解決 /
	// buildConfigField "korlibs.time..." のような文字列内 libs. は拾わない
	wantLegacy := []skeleton.Lib{
		{Coordinate: "com.google.code.gson:gson", Version: "$gsonVersion", Kind: "implementation"},
		{Coordinate: "com.google.guava:guava", Version: "33.0.0", Kind: "implementation"},
		{Coordinate: "com.squareup.okhttp3:okhttp", Version: "4.11.0", Kind: "implementation"},
	}
	if got := byName[":legacy-lib"]; !reflect.DeepEqual(got, wantLegacy) {
		t.Errorf(":legacy-lib libs = %v, want %v", got, wantLegacy)
	}
}
