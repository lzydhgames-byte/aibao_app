import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'api/api_client.dart';
import 'api/secure_token_storage.dart';
import 'router.dart';
import 'state/auth_state.dart';
import 'theme.dart';

/// Global ApiClient provider — wired with the Flutter-plugin-backed
/// secure storage in the app entrypoint. Tests override this with an
/// ApiClient backed by `InMemoryTokenStorage`.
///
/// The `onUnauthorized` callback bridges a dio response interceptor (Task 3)
/// to the auth notifier so that any non-/auth 401 bounces the user back to
/// /login. We defer the logout call with `Future.microtask` because:
///   1. dio's response interceptor runs inside the active HTTP completion;
///      synchronously mutating Riverpod state from there can racily
///      reenter the same request stack on some platforms;
///   2. it gives go_router's redirect listener a clean tick to re-evaluate.
/// Mutable cell tracking the latest auth state. Updated by AuthNotifier on
/// every state transition (see auth_state.dart). The 401 interceptor reads
/// this synchronously WITHOUT going through Riverpod — avoiding circular
/// dependency between apiClientProvider and authProvider at construction
/// time (authProvider's notifier itself depends on apiClientProvider).
class AuthSnapshot {
  bool isAuthenticated = false;
}

final authSnapshotProvider = Provider<AuthSnapshot>((_) => AuthSnapshot());

final Provider<ApiClient> apiClientProvider = Provider<ApiClient>((ref) {
  final snapshot = ref.read(authSnapshotProvider);
  return ApiClient(
    storage: SecureTokenStorage(),
    onUnauthorized: () {
      Future.microtask(() {
        // Only react to 401 if we currently *think* we are authenticated.
        // Otherwise we create a feedback loop:
        //   childProvider.build() → GET /children → 401 → logout →
        //   invalidate childProvider → re-build → loop.
        if (!snapshot.isAuthenticated) return;
        // Looking up the notifier here is safe because by the time a 401
        // fires for a non-/auth path, authProvider has already been built
        // (it built apiClientProvider's lazy dep tree on app start).
        ref.read(authProvider.notifier).logout();
      });
    },
  );
});

void main() {
  runApp(const ProviderScope(child: AibaoApp()));
}

class AibaoApp extends ConsumerWidget {
  const AibaoApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final router = ref.watch(routerProvider);
    return MaterialApp.router(
      title: '爱宝',
      theme: buildLightTheme(),
      routerConfig: router,
    );
  }
}
