package moduledep

import (
	"reflect"
	"testing"

	"github.com/kr9ly/skeleton/skeleton"
)

func TestGradleExtract(t *testing.T) {
	g := &gradleProvider{}
	root := "testdata/gradle-sample"

	if !g.Detect(root) {
		t.Fatal("Detect should find settings.gradle.kts")
	}

	graph, err := g.Extract(root)
	if err != nil {
		t.Fatal(err)
	}

	if graph.System != "gradle" {
		t.Errorf("System = %q, want gradle", graph.System)
	}

	var names []string
	byName := make(map[string]skeleton.Module)
	for _, m := range graph.Modules {
		names = append(names, m.Name)
		byName[m.Name] = m
	}

	wantNames := []string{":app", ":core:model", ":core:ui", ":feature:home", ":legacy-lib", ":util"}
	if !reflect.DeepEqual(names, wantNames) {
		t.Fatalf("modules = %v, want %v", names, wantNames)
	}

	// projectDir 上書き
	if got := byName[":legacy-lib"].Dir; got != "legacy" {
		t.Errorf(":legacy-lib dir = %q, want legacy", got)
	}

	// kts: project() / project(path =) / typesafe accessor / コメント除去
	wantApp := []skeleton.ModuleDep{
		{Target: ":core:model", Kind: "implementation"},
		{Target: ":core:model", Kind: "testImplementation"},
		{Target: ":core:ui", Kind: "implementation"},
		{Target: ":feature:home", Kind: "implementation"},
	}
	if got := byName[":app"].Deps; !reflect.DeepEqual(got, wantApp) {
		t.Errorf(":app deps = %v, want %v", got, wantApp)
	}

	// typesafe accessor の kebab-case 逆引き（projects.legacyLib → :legacy-lib）
	wantUI := []skeleton.ModuleDep{
		{Target: ":core:model", Kind: "api"},
		{Target: ":legacy-lib", Kind: "implementation"},
	}
	if got := byName[":core:ui"].Deps; !reflect.DeepEqual(got, wantUI) {
		t.Errorf(":core:ui deps = %v, want %v", got, wantUI)
	}

	// groovy: 括弧なし呼び出し
	wantHome := []skeleton.ModuleDep{
		{Target: ":core:model", Kind: "implementation"},
		{Target: ":core:ui", Kind: "api"},
	}
	if got := byName[":feature:home"].Deps; !reflect.DeepEqual(got, wantHome) {
		t.Errorf(":feature:home deps = %v, want %v", got, wantHome)
	}

	// apply from の共有スクリプトを再帰的に辿る（rootProject.file → ${rootDir}）
	wantModel := []skeleton.ModuleDep{
		{Target: ":legacy-lib", Kind: "implementation"},
		{Target: ":util", Kind: "compileOnly"},
	}
	if got := byName[":core:model"].Deps; !reflect.DeepEqual(got, wantModel) {
		t.Errorf(":core:model deps = %v, want %v", got, wantModel)
	}

	// build ファイルなしのモジュールは依存ゼロ
	if got := byName[":util"].Deps; len(got) != 0 {
		t.Errorf(":util deps = %v, want empty", got)
	}
}
