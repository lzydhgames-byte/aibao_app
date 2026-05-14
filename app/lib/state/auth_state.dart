import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/api_client.dart';
import '../api/api_exception.dart';
import '../api/models/user.dart';
import '../main.dart' show apiClientProvider, authSnapshotProvider;
import 'child_state.dart' show childProvider;

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
  final Ref _ref;
  AuthNotifier(this._api, this._ref) : super(const AuthInitial()) {
    _bootstrap();
  }

  /// Mirror state transitions into [authSnapshotProvider] so the
  /// apiClientProvider's 401 interceptor can synchronously decide whether
  /// to bounce the user — WITHOUT calling ref.read(authProvider) inside
  /// apiClientProvider (which would create a real circular dependency at
  /// construction time: apiClient → authProvider → AuthNotifier needs api).
  @override
  set state(AuthState value) {
    super.state = value;
    _ref.read(authSnapshotProvider).isAuthenticated = value is AuthAuthenticated;
  }

  /// Invalidate every provider that holds user-scoped state. Called on
  /// login (new user session may not match previously cached data) and
  /// logout (clear stale data immediately so login screen doesn't flash
  /// the previous user's child/storylines).
  ///
  /// childProvider is the only one whose state must NOT survive a session
  /// change. heartbeat/storyList/bootstrap are FutureProvider.family keyed
  /// on childId so they reset automatically when child.id changes (different
  /// family key = different provider instance).
  void _resetUserScopedState() {
    _ref.invalidate(childProvider);
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
      // Wipe user-scoped caches BEFORE flipping state — otherwise widgets
      // listening to authProvider may rebuild with the OLD user's child/
      // storyline data still in childProvider's cache.
      _resetUserScopedState();
      state = AuthAuthenticated(result.user);
    } on ApiException catch (e) {
      state = AuthError(e.userMsg);
      throw Exception(e.userMsg);
    }
  }

  Future<void> logout() async {
    await _api.logout();
    _resetUserScopedState();
    state = const AuthUnauthenticated();
  }
}

final authProvider = StateNotifierProvider<AuthNotifier, AuthState>(
  (ref) => AuthNotifier(ref.watch(apiClientProvider), ref),
);
