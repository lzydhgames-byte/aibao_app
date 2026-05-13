import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'api/api_client.dart';
import 'api/secure_token_storage.dart';
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

class AibaoApp extends StatelessWidget {
  const AibaoApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: '爱宝',
      theme: buildLightTheme(),
      home: const _PlaceholderHome(),
    );
  }
}

class _PlaceholderHome extends StatelessWidget {
  const _PlaceholderHome();

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('爱宝')),
      body: const Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Text('🐼', style: TextStyle(fontSize: 80)),
            SizedBox(height: 16),
            Text('Plan 9-A 脚手架就绪', style: TextStyle(fontSize: 20)),
            SizedBox(height: 8),
            Text('Task 6+ 会替换为真实登录屏'),
          ],
        ),
      ),
    );
  }
}
