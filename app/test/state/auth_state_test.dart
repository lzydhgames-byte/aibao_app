import 'package:flutter_test/flutter_test.dart';
import 'package:aibao_app/state/auth_state.dart';
import 'package:aibao_app/api/models/user.dart';

void main() {
  group('AuthState sealed hierarchy', () {
    test('AuthInitial is const', () {
      expect(const AuthInitial(), isA<AuthState>());
    });

    test('AuthUnauthenticated is const', () {
      expect(const AuthUnauthenticated(), isA<AuthState>());
    });

    test('AuthAuthenticated holds user', () {
      final u = User(id: 1, nickname: 'x', subscriptionTier: 'free');
      final s = AuthAuthenticated(u);
      expect(s.user.id, 1);
    });

    test('AuthError holds message', () {
      const e = AuthError('boom');
      expect(e.message, 'boom');
    });
  });
}
