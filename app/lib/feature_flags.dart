/// Plan 11A feature flags — compile-time switches for the outline preview UX.
///
/// Pass via `flutter build apk --dart-define=OUTLINE_ENABLED=false` to
/// emergency-rollback to the legacy generate UI.
class FeatureFlags {
  static const bool outlineEnabled = bool.fromEnvironment(
    'OUTLINE_ENABLED',
    defaultValue: true,
  );
}
