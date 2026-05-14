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
final Provider<ApiClient> apiClientProvider = Provider<ApiClient>((ref) {
  return ApiClient(
    storage: SecureTokenStorage(),
    onUnauthorized: () {
      Future.microtask(() {
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
