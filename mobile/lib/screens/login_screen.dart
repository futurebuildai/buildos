import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../l10n/app_localizations.dart';
import '../providers/app_providers.dart';
import '../services/api_error.dart';
import '../theme/app_theme.dart';

/// Native email/password sign-in (UX_AUTH_ONBOARDING §1). Sign-in needs the
/// network, so the button is disabled offline with an explicit hint — unlike
/// the rest of the app, which is offline-first.
class LoginScreen extends ConsumerStatefulWidget {
  const LoginScreen({super.key});

  @override
  ConsumerState<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends ConsumerState<LoginScreen> {
  final _formKey = GlobalKey<FormState>();
  final _email = TextEditingController();
  final _password = TextEditingController();
  bool _submitting = false;
  String? _error;

  @override
  void dispose() {
    _email.dispose();
    _password.dispose();
    super.dispose();
  }

  Future<void> _submit(AppLocalizations l10n) async {
    if (!(_formKey.currentState?.validate() ?? false)) return;
    setState(() {
      _submitting = true;
      _error = null;
    });
    try {
      await ref
          .read(authControllerProvider.notifier)
          .login(_email.text.trim(), _password.text);
    } on ApiError catch (e) {
      setState(
        () => _error = e.status == 401 ? l10n.invalidCredentials : e.message,
      );
    } catch (_) {
      setState(() => _error = l10n.genericError);
    } finally {
      if (mounted) setState(() => _submitting = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final online = ref.watch(onlineProvider).value ?? true;

    return Scaffold(
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 420),
              child: Form(
                key: _formKey,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Text(
                      l10n.loginTitle,
                      style: const TextStyle(
                        fontSize: 26,
                        fontWeight: FontWeight.w700,
                        color: FbColors.textPrimary,
                      ),
                    ),
                    const SizedBox(height: 8),
                    Text(
                      l10n.loginSubtitle,
                      style: const TextStyle(color: FbColors.textSecondary),
                    ),
                    const SizedBox(height: 32),
                    TextFormField(
                      controller: _email,
                      keyboardType: TextInputType.emailAddress,
                      autofillHints: const [AutofillHints.email],
                      textInputAction: TextInputAction.next,
                      decoration: InputDecoration(labelText: l10n.emailLabel),
                      validator: (v) => (v == null || !v.contains('@'))
                          ? l10n.emailLabel
                          : null,
                    ),
                    const SizedBox(height: FbSizes.gap),
                    TextFormField(
                      controller: _password,
                      obscureText: true,
                      autofillHints: const [AutofillHints.password],
                      textInputAction: TextInputAction.done,
                      decoration: InputDecoration(
                        labelText: l10n.passwordLabel,
                      ),
                      validator: (v) =>
                          (v == null || v.isEmpty) ? l10n.passwordLabel : null,
                      onFieldSubmitted: (_) {
                        if (online && !_submitting) _submit(l10n);
                      },
                    ),
                    if (_error != null) ...[
                      const SizedBox(height: FbSizes.gap),
                      Text(
                        _error!,
                        style: const TextStyle(color: FbColors.safetyRed),
                      ),
                    ],
                    const SizedBox(height: 24),
                    FilledButton(
                      onPressed: (!online || _submitting)
                          ? null
                          : () => _submit(l10n),
                      child: _submitting
                          ? const SizedBox(
                              width: 22,
                              height: 22,
                              child: CircularProgressIndicator(
                                strokeWidth: 2,
                                color: FbColors.deepSpace,
                              ),
                            )
                          : Text(l10n.signIn),
                    ),
                    if (!online) ...[
                      const SizedBox(height: FbSizes.gapSmall),
                      Text(
                        l10n.offlineSignInDisabled,
                        textAlign: TextAlign.center,
                        style: const TextStyle(color: FbColors.amber),
                      ),
                    ],
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
