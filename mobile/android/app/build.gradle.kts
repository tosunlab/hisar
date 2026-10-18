plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
}

val hisarKit = rootProject.layout.projectDirectory.file("../build/HisarKit.aar")

android {
    namespace = "com.tryhisar.hisar"
    compileSdk = 37

    defaultConfig {
        applicationId = "com.tryhisar.hisar"
        minSdk = 29
        targetSdk = 37
        versionCode = providers.gradleProperty("hisar.versionCode").get().toInt()
        versionName = providers.gradleProperty("hisar.versionName").get()
        testInstrumentationRunner = "com.tryhisar.hisar.HisarTestRunner"
    }

    buildTypes {
        debug {
            applicationIdSuffix = ".debug"
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    // Each test runs in its own process after its app data and Keystore keys are cleared, as -HisarResetState does on iOS.
    testOptions {
        execution = "ANDROIDX_TEST_ORCHESTRATOR"
        animationsDisabled = true
    }
    defaultConfig.testInstrumentationRunnerArguments["clearPackageData"] = "true"
    defaultConfig.testInstrumentationRunnerArguments["useTestStorageService"] = "true"
}

tasks.named("preBuild") {
    val hisarKitFile = hisarKit.asFile
    doFirst {
        check(hisarKitFile.exists()) { "$hisarKitFile is missing; build it with make build-android-aar" }
    }
}

dependencies {
    implementation(project(":model"))
    implementation(files(hisarKit))
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.material3)
    implementation(libs.compose.ui)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.fragment.ktx)
    implementation(libs.androidx.lifecycle.process)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.compose.material.icons.extended)
    implementation(libs.androidx.biometric)
    implementation(libs.kotlinx.coroutines.core)
    implementation(libs.androidx.autofill)
    implementation(libs.androidx.credentials)

    testImplementation(testFixtures(project(":model")))
    testImplementation(libs.kotlin.test.junit)
    androidTestImplementation(testFixtures(project(":model")))
    androidTestImplementation(platform(libs.compose.bom))
    androidTestImplementation(libs.compose.ui.test.junit4)
    androidTestImplementation(libs.androidx.test.ext.junit)
    androidTestImplementation(libs.androidx.test.runner)
    // Compose's ui-test brings Espresso 3.5, which calls InputManager.getInstance, gone on API 37.
    androidTestImplementation(libs.androidx.test.espresso.core)
    androidTestImplementation(libs.androidx.test.services.storage)
    androidTestUtil(libs.androidx.test.orchestrator)
    androidTestUtil(libs.androidx.test.services)
    androidTestImplementation(libs.androidx.test.uiautomator)
    androidTestImplementation(libs.androidx.fragment.testing)
    debugImplementation(libs.compose.ui.test.manifest)
    debugImplementation(libs.androidx.fragment.testing.manifest)
}
