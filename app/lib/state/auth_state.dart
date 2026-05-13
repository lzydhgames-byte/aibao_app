import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/api_client.dart';
import '../api/api_exception.dart';
import '../api/models/user.dart';
import '../main.dart' show apiClientProvider;

/// AuthState represents the user's session lifecycle:
/// - initial: unknown (app just opened, checking persisted token)
/// - unauthenticated: no token or token invalid
/// - authenticated: have token + user info loaded
/// - error: bootstrap or login failed
sealed class AuthState {
  const AuthState();
}

class AuthInitial extends AuthState {
  const AuthInitial();
}

class AuthUnauthenticated extends AuthState {
  const AuthUnauthenticated();
}

class AuthAuthenticated extends AuthState {
  final User user;
  const AuthAuthenticated(this.user);
}

class AuthError extends AuthState {
  final String message;
  const AuthError(this.message);
}

class AuthNotifier extends StateNotifier<AuthState> {
  final ApiClient _api;
  AuthNotifier(this._api) : super(const AuthInitial()) {
    _bootstrap();
  }

  /// Check persisted token on app start; verify with /me; emit state.
  Future<void> _bootstrap() async {
    try {
      final token = await _api.currentToken();
      if (token == null || token.isEmpty) {
        state = const AuthUnauthenticated();
        return;
      }
      final me = await _api.getMe();
      state = AuthAuthenticated(me);
    } on ApiException catch (e) {
      if (e.statusCode == 401) {
        await _api.logout();
        state = const AuthUnauthenticated();
      } else {
        state = AuthError(e.userMsg);
      }
    } catch (e) {
      state = AuthError('登录状态加载失败: $e');
    }
  }

  /// Step 1 of login: send SMS code to phone.
  Future<void> sendSmsCode(String phone) async {
    try {
      await _api.sendSmsCode(phone);
    } on ApiException catch (e) {
      throw Exception(e.userMsg);
    }
  }

  /// Step 2 of login: submit phone + code. On success, store user.
  Future<void> loginOrRegister({
    required String phone,
    required String code,
    String nickname = '',
  }) async {
    try {
      final result = await _api.loginOrRegister(
        phone: phone,
        code: code,
        nickname: nickname,
      );
      state = AuthAuthenticated(result.user);
    } on ApiException catch (e) {
      state = AuthError(e.userMsg);
      throw Exception(e.userMsg);
    }
  }

  Future<void> logout() async {
    await _api.logout();
    state = const AuthUnauthenticated();
  }
}

final authProvider = StateNotifierProvider<AuthNotifier, AuthState>(
  (ref) => AuthNotifier(ref.watch(apiClientProvider)),
);
