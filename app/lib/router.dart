import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'screens/login_screen.dart';
import 'screens/home_screen.dart';
import 'screens/generate_screen.dart';
import 'screens/player_screen.dart';
import 'state/auth_state.dart';

/// Builds the app's GoRouter. Wires Riverpod auth state into route guards via
/// a ChangeNotifier bridge so that login / logout transitions trigger a
/// redirect re-evaluation automatically.
GoRouter buildRouter(Ref ref) {
  final listenable = _AuthListenable(ref);
  return GoRouter(
    initialLocation: '/',
    refreshListenable: listenable,
    redirect: (ctx, st) {
      final auth = ref.read(authProvider);
      // Bootstrap still in flight — stay on the loading splash.
      if (auth is AuthInitial) {
        return st.matchedLocation == '/' ? null : '/';
      }
      final loggingIn = st.matchedLocation == '/login';
      // Not signed in → everywhere except /login pushes to /login.
      if (auth is AuthUnauthenticated || auth is AuthError) {
        return loggingIn ? null : '/login';
      }
      // Signed in: if user is on splash or /login, send to /home.
      if (auth is AuthAuthenticated) {
        if (st.matchedLocation == '/' || loggingIn) return '/home';
      }
      return null;
    },
    routes: [
      GoRoute(
        path: '/',
        builder: (_, __) => const _Splash(),
      ),
      GoRoute(
        path: '/login',
        builder: (_, __) => const LoginScreen(),
      ),
      GoRoute(
        path: '/home',
        builder: (_, __) => const HomeScreen(),
      ),
      GoRoute(
        path: '/generate',
        builder: (_, __) => const GenerateScreen(),
      ),
      GoRoute(
        path: '/player/:id',
        builder: (_, st) =>
            PlayerScreen(storyId: int.parse(st.pathParameters['id']!)),
      ),
    ],
  );
}

class _Splash extends StatelessWidget {
  const _Splash();
  @override
  Widget build(BuildContext context) {
    return const Scaffold(
      body: Center(child: CircularProgressIndicator()),
    );
  }
}

/// Bridge: turns Riverpod's authProvider into a Listenable that GoRouter can
/// subscribe to via refreshListenable. Without this, GoRouter would not
/// re-evaluate redirect on login/logout transitions.
class _AuthListenable extends ChangeNotifier {
  _AuthListenable(this._ref) {
    _ref.listen<AuthState>(authProvider, (_, __) => notifyListeners());
  }
  final Ref _ref;
}

final routerProvider = Provider<GoRouter>((ref) => buildRouter(ref));
