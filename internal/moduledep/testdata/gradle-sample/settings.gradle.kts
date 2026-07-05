rootProject.name = "gradle-sample"

// コメント内の include(":ignored") は拾われない
include(":app", ":core:model")
include(
    ":core:ui",
    ":feature:home",
)
include(":legacy-lib")
include(":util")
project(":legacy-lib").projectDir = file("legacy")

includeBuild("build-logic")
