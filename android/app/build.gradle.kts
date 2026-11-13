plugins {
    id("com.android.application") version "9.2.1"
    id("org.jetbrains.kotlin.plugin.compose") version "2.2.10"
    id("com.google.devtools.ksp") version "2.3.13"
}

val signingVariables = listOf(
    "VANILLAPAY_KEYSTORE_PATH", "VANILLAPAY_KEYSTORE_PASSWORD",
    "VANILLAPAY_KEY_ALIAS", "VANILLAPAY_KEY_PASSWORD",
)
val releaseSigning = signingVariables.associateWith { providers.environmentVariable(it).orNull }
val defaultBase = providers.gradleProperty("vanillapayBaseUrl").orElse("").get()
require(defaultBase.all { it.code in 33..126 && it != '"' && it != '\\' }) {
    "vanillapayBaseUrl must be an ASCII URL without whitespace, quotes or backslashes"
}

val monitorVersion = "1.1.1"
val (major, minor, patch) = monitorVersion.split('.').map(String::toInt)
require(minor in 0..99 && patch in 0..99) { "versionName requires minor and patch below 100" }
val monitorVersionCode = major * 10_000 + minor * 100 + patch

android {
    namespace = "com.vanillapay.monitor"
    compileSdk = 37
    defaultConfig {
        applicationId = "com.vanillapay.monitor"
        minSdk = 26
        targetSdk = 37
        versionName = monitorVersion
        versionCode = monitorVersionCode
        buildConfigField("String", "DEFAULT_BASE_URL", "\"$defaultBase\"")
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_21
        targetCompatibility = JavaVersion.VERSION_21
    }
    signingConfigs {
        if (releaseSigning.values.none { it.isNullOrBlank() }) {
            create("production") {
                storeFile = file(releaseSigning.getValue("VANILLAPAY_KEYSTORE_PATH")!!)
                storePassword = releaseSigning.getValue("VANILLAPAY_KEYSTORE_PASSWORD")
                keyAlias = releaseSigning.getValue("VANILLAPAY_KEY_ALIAS")
                keyPassword = releaseSigning.getValue("VANILLAPAY_KEY_PASSWORD")
            }
        }
    }
    buildTypes {
        release {
            signingConfig = signingConfigs.findByName("production")
        }
    }
    testOptions.unitTests.isIncludeAndroidResources = true
}

androidComponents.onVariants(androidComponents.selector().withBuildType("release")) { variant ->
    variant.outputs.forEach { output ->
        output.outputFileName.set(output.versionName.map { "vanillapay-monitor-$it.apk" })
    }
}

val verifyReleaseSigning by tasks.registering {
    doLast {
        val missing = releaseSigning.filterValues { it.isNullOrBlank() }.keys
        check(missing.isEmpty()) { "Release signing requires: ${missing.joinToString()}" }
        check(file(releaseSigning.getValue("VANILLAPAY_KEYSTORE_PATH")!!).isFile) { "Release keystore file does not exist" }
    }
}
tasks.matching { it.name == "preReleaseBuild" }.configureEach { dependsOn(verifyReleaseSigning) }

val screenshotsEnabled = providers.gradleProperty("monitorScreenshots").isPresent
val screenshotFolder = layout.projectDirectory.dir("../../.artifacts/android-screens")
tasks.withType<Test>().configureEach {
    if (screenshotsEnabled) {
        systemProperty("monitor.screenshots", screenshotFolder.asFile.absolutePath)
        systemProperty("roborazzi.test.record", "true")
        systemProperty("robolectric.pixelCopyRenderMode", "hardware")
        outputs.dir(screenshotFolder)
    }
}

dependencies {
    val composeBom = platform("androidx.compose:compose-bom:2026.03.00")
    implementation(composeBom)
    implementation("androidx.activity:activity-compose:1.12.4")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.ui:ui")
    implementation("androidx.core:core-ktx:1.17.0")
    implementation("androidx.datastore:datastore-preferences:1.2.1")
    implementation("androidx.room:room-ktx:2.8.4")
    implementation("androidx.work:work-runtime-ktx:2.11.1")
    ksp("androidx.room:room-compiler:2.8.4")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.10.2")
    val cameraX = "1.5.3"
    implementation("androidx.camera:camera-camera2:$cameraX")
    implementation("androidx.camera:camera-lifecycle:$cameraX")
    implementation("androidx.camera:camera-view:$cameraX")
    implementation("com.google.zxing:core:3.5.4")
    implementation("com.squareup.okhttp3:okhttp:4.12.0")

    debugImplementation("androidx.compose.ui:ui-test-manifest")
    testImplementation(composeBom)
    testImplementation("androidx.compose.ui:ui-test-junit4")
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.robolectric:robolectric:4.16.1")
    testImplementation("io.github.takahirom.roborazzi:roborazzi-compose:1.54.0")
    testImplementation("com.squareup.okhttp3:mockwebserver:4.12.0")
}
