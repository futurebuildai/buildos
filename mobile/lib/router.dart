import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import 'providers/app_providers.dart';
import 'screens/home_shell.dart';
import 'screens/login_screen.dart';

/// App router. The redirect is driven by [authControllerProvider]: a resolved
/// user lands in the field shell, otherwise we send them to /login. A failed
/// token refresh flips auth to logged-out, which this redirect catches.
final routerProvider = Provider<GoRouter>((ref) {
  final notifier = _AuthRouterNotifier(ref);
  ref.onDispose(notifier.dispose);

  return GoRouter(
    initialLocation: '/',
    refreshListenable: notifier,
    redirect: (context, state) {
      final auth = ref.read(authControllerProvider);
      // Hold on the initial cached-user resolve to avoid a login flash.
      if (auth.isLoading && !auth.hasValue) return null;

      final loggedIn = auth.value != null;
      final atLogin = state.matchedLocation == '/login';

      if (!loggedIn) return atLogin ? null : '/login';
      if (atLogin || state.matchedLocation == '/') return '/home';
      return null;
    },
    routes: [
      GoRoute(path: '/', redirect: (_, _) => '/home'),
      GoRoute(path: '/login', builder: (_, _) => const LoginScreen()),
      GoRoute(path: '/home', builder: (_, _) => const HomeShell()),
    ],
  );
});

/// Bridges the Riverpod auth state to a [Listenable] so go_router re-evaluates
/// its redirect on every login/logout/session-expiry.
class _AuthRouterNotifier extends ChangeNotifier {
  _AuthRouterNotifier(Ref ref) {
    _sub = ref.listen(
      authControllerProvider,
      (_, _) => notifyListeners(),
      fireImmediately: false,
    );
  }

  late final ProviderSubscription _sub;

  @override
  void dispose() {
    _sub.close();
    super.dispose();
  }
}
