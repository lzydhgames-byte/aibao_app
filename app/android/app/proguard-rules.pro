# Plan 10 release APK proguard rules.
# Keeps just_audio's ExoPlayer reflection targets so R8 minify doesn't strip them
# and break audio playback on release builds. Plan 9-A originally hit this as a
# NullPointerException on ColorOS; the symptom is "audio prepares then errors out
# immediately on play".

# ExoPlayer / Media3 (used by just_audio)
-keep class com.google.android.exoplayer2.** { *; }
-keep interface com.google.android.exoplayer2.** { *; }
-keep class androidx.media3.** { *; }
-keep interface androidx.media3.** { *; }
-dontwarn com.google.android.exoplayer2.**
-dontwarn androidx.media3.**

# Flutter plugin reflection
-keep class io.flutter.plugin.** { *; }
-keep class io.flutter.plugins.** { *; }

# Dio uses reflection for some JSON paths in error responses
-keep class dio.** { *; }
-dontwarn dio.**

# Keep generic signatures (some libs need them at runtime)
-keepattributes Signature
-keepattributes *Annotation*
-keepattributes EnclosingMethod
-keepattributes InnerClasses
