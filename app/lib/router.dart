import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'api/models/child.dart';
import 'screens/bootstrap_screen.dart';
import 'screens/create_child_screen.dart';
import 'screens/login_screen.dart';
import 'screens/home_screen.dart';
import 'screens/generate_screen.dart';
import 'screens/player_screen.dart';
import 'state/auth_state.dart';
import 'state/child_state.dart';

/// Builds the app's GoRouter. Wires Riverpod auth + child state into route
/// guards via a ChangeNotifier bridge so login / logout / child-created
/// transitions trigger a redirect re-evaluation automatically.
GoRouter buildRouter(Ref ref) {
  final listenable = _AppListenable(ref);
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
      // Signed in: gate on child profile state.
      if (auth is AuthAuthenticated) {
        final childAsync = ref.read(childProvider);
        // Still loading the child — keep user on splash to avoid flicker.
        if (childAsync.isLoading) {
          return st.matchedLocation == '/' ? null : '/';
        }
        final child = childAsync.valueOrNull;
        final atCreateChild = st.matchedLocation == '/onboarding/create-child';
        if (child == null) {
          // Must create child before anything else.
          return atCreateChild ? null : '/onboarding/create-child';
        }
        // Child exists; if user is on splash / login / create-child, go home.
        if (st.matchedLocation == '/' || loggingIn || atCreateChild) {
          return '/home';
        }
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
        path: '/onboarding/create-child',
        builder: (_, __) => const CreateChildScreen(),
      ),
      GoRoute(
        path: '/bootstrap',
        builder: (_, __) => const BootstrapScreen(),
      ),
      GoRoute(
        path: '/home',
        builder: (_, __) => const HomeScreen(),
      ),
      GoRoute(
        path: '/generate',
        builder: (_, st) {
          final s = st.uri.queryParameters['storyline_id'];
          return GenerateScreen(
            storylineId: s == null ? null : int.tryParse(s),
          );
        },
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

/// Bridge: turns Riverpod auth + child providers into a Listenable that
/// GoRouter can subscribe to via refreshListenable. Without this, GoRouter
/// would not re-evaluate redirect on login/logout or child-created
/// transitions.
class _AppListenable extends ChangeNotifier {
  _AppListenable(this._ref) {
    _ref.listen<AuthState>(authProvider, (_, __) => notifyListeners());
    _ref.listen<AsyncValue<Child?>>(
      childProvider,
      (_, __) => notifyListeners(),
    );
  }
  final Ref _ref;
}

final routerProvider = Provider<GoRouter>((ref) => buildRouter(ref));
