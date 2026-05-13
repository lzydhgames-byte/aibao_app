import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'api/api_client.dart';
import 'api/secure_token_storage.dart';
import 'router.dart';
import 'theme.dart';

/// Global ApiClient provider — wired with the Flutter-plugin-backed
/// secure storage in the app entrypoint. Tests override this with an
/// ApiClient backed by `InMemoryTokenStorage`.
final apiClientProvider = Provider<ApiClient>(
  (ref) => ApiClient(storage: SecureTokenStorage()),
);

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
