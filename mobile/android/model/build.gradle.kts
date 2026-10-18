import org.jetbrains.kotlin.gradle.dsl.JvmTarget

plugins {
    `java-test-fixtures`
    alias(libs.plugins.kotlin.jvm)
    alias(libs.plugins.kotlin.serialization)
}

java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}

kotlin {
    compilerOptions {
        jvmTarget = JvmTarget.JVM_17
    }
}

dependencies {
    api(libs.kotlinx.serialization.json)
    api(libs.kotlinx.io.bytestring)
    api(libs.kotlinx.coroutines.core)
    testFixturesApi(libs.kotlinx.coroutines.test)
    testImplementation(libs.kotlin.test.junit)
}

// IosTestParityTest reads the iPhone app's tests and every Android test's source, and PrivilegedBrowsersTests the app's
// privileged-browser list; tests run in this module's directory.
tasks.test {
    val repositoryDirectory = rootProject.layout.projectDirectory.dir("../..")
    systemProperty("hisar.repositoryRoot", "../../..")
    inputs.files(repositoryDirectory.dir("mobile/ios/Tests").asFileTree.matching { include("**/*.swift") })
        .withPropertyName("iosTests").withPathSensitivity(PathSensitivity.RELATIVE)
    inputs.files(rootProject.layout.projectDirectory.asFileTree.matching { include("*/src/test/**/*.kt", "*/src/androidTest/**/*.kt") })
        .withPropertyName("androidTests").withPathSensitivity(PathSensitivity.RELATIVE)
    inputs.file(rootProject.layout.projectDirectory.file("app/src/main/assets/privileged_browsers.json"))
        .withPropertyName("privilegedBrowsers").withPathSensitivity(PathSensitivity.RELATIVE)
}