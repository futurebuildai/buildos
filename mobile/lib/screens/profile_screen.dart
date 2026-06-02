import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../l10n/app_localizations.dart';
import '../providers/app_providers.dart';
import '../theme/app_theme.dart';

/// Profile + sign-out. Read-only identity (display name / email / role); the
/// role label is humanized from the backend's RBAC token.
class ProfileScreen extends ConsumerWidget {
  const ProfileScreen({super.key});

  static String _humanRole(String role) => switch (role) {
    'owner' => 'Owner',
    'admin' => 'Admin',
    'superintendent' => 'Superintendent',
    'field_worker' => 'Field Worker',
    _ => role,
  };

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final user = ref.watch(authControllerProvider).value;

    return Scaffold(
      appBar: AppBar(title: Text(l10n.profileTitle)),
      body: ListView(
        padding: const EdgeInsets.all(FbSizes.gap),
        children: [
          if (user != null) ...[
            Text(
              user.displayName.isEmpty ? user.email : user.displayName,
              style: const TextStyle(
                fontSize: 22,
                fontWeight: FontWeight.w700,
                color: FbColors.textPrimary,
              ),
            ),
            const SizedBox(height: 4),
            Text(
              user.email,
              style: const TextStyle(color: FbColors.textSecondary),
            ),
            const SizedBox(height: FbSizes.gap),
            Card(
              child: ListTile(
                title: Text(l10n.roleLabel),
                trailing: Text(_humanRole(user.role)),
              ),
            ),
            Card(
              child: ListTile(
                title: Text(l10n.languageLabel),
                trailing: Text(user.locale.toUpperCase()),
              ),
            ),
          ],
          const SizedBox(height: 24),
          OutlinedButton.icon(
            onPressed: () async {
              await ref.read(authControllerProvider.notifier).logout();
            },
            icon: const Icon(Icons.logout, color: FbColors.safetyRed),
            label: Text(
              l10n.signOut,
              style: const TextStyle(color: FbColors.safetyRed),
            ),
          ),
        ],
      ),
    );
  }
}
