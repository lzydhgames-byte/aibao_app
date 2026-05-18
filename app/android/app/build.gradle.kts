plugins {
    id("com.android.application")
    id("kotlin-android")
    // The Flutter Gradle Plugin must be applied after the Android and Kotlin Gradle plugins.
    id("dev.flutter.flutter-gradle-plugin")
}

android {
    namespace = "com.aibao.aibao_app"
    // Pinned by Plan 9-A Task 11 — flutter_secure_storage requires SDK 36;
    // just_audio / audio_session / path_provider_android need NDK 27.0.12077973.
    compileSdk = 36
    ndkVersion = "27.0.12077973"

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_11
        targetCompatibility = JavaVersion.VERSION_11
    }

    kotlinOptions {
        jvmTarget = JavaVersion.VERSION_11.toString()
    }

    defaultConfig {
        // TODO: Specify your own unique Application ID (https://developer.android.com/studio/build/application-id.html).
        applicationId = "com.aibao.aibao_app"
        // Pinned to 23 — flutter_secure_storage requires minSdk >= 23.
        // Android 6.0 (API 23) covers 99%+ of active devices; safe baseline.
        minSdk = 23
        targetSdk = flutter.targetSdkVersion
        versionCode = flutter.versionCode
        versionName = flutter.versionName
    }

    buildTypes {
        release {
            // Plan 10 MVP: signing with debug key — friends can install directly,
            // bypassing Play Store. Replace before Plan 11/12 app-store release.
            signingConfig = signingConfigs.getByName("debug")
            // Plan 10: enable R8 minification + proguard rules for just_audio /
            // ExoPlayer reflection targets. Without rules audio breaks at runtime
            // (see proguard-rules.pro for the canonical fix list).
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
        }
    }
}

flutter {
    source = "../.."
}
