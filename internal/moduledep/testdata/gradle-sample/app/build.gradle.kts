plugins {
    id("com.android.application")
}

dependencies {
    implementation(project(":core:model"))
    implementation(project(path = ":core:ui"))
    implementation(projects.feature.home)
    testImplementation(project(":core:model"))
    /* block comment: api(project(":core:ui")) */
    // line comment: implementation(project(":legacy-lib"))
    implementation("io.ktor:ktor-client-core:2.3.0") // https://ktor.io
}
