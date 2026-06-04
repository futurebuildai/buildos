import 'dart:convert';

import 'package:buildos_field/models/user.dart';
import 'package:buildos_field/services/token_store.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';

/// An in-memory [FlutterSecureStorage] — overrides the three methods
/// [TokenStore] touches (read / write / deleteAll) so no Keychain/Keystore
/// platform channel is required. The default super() options are unused.
class _MemSecureStorage extends FlutterSecureStorage {
  _MemSecureStorage();

  final Map<String, String> data = {};

  @override
  Future<void> write({
    required String key,
    required String? value,
    AppleOptions? iOptions,
    AndroidOptions? aOptions,
    LinuxOptions? lOptions,
    WebOptions? webOptions,
    AppleOptions? mOptions,
    WindowsOptions? wOptions,
  }) async {
    if (value == null) {
      data.remove(key);
    } else {
      data[key] = value;
    }
  }

  @override
  Future<String?> read({
    required String key,
    AppleOptions? iOptions,
    AndroidOptions? aOptions,
    LinuxOptions? lOptions,
    WebOptions? webOptions,
    AppleOptions? mOptions,
    WindowsOptions? wOptions,
  }) async => data[key];

  @override
  Future<void> deleteAll({
    AppleOptions? iOptions,
    AndroidOptions? aOptions,
    LinuxOptions? lOptions,
    WebOptions? webOptions,
    AppleOptions? mOptions,
    WindowsOptions? wOptions,
  }) async => data.clear();
}

TokenPair _pair({
  String access = 'a-tok',
  String refresh = 'r-tok',
  int expiresIn = 900,
}) => TokenPair(
  accessToken: access,
  refreshToken: refresh,
  expiresIn: expiresIn,
  user: const User(
    id: 'u1',
    orgId: 'org1',
    email: 'field@example.com',
    displayName: 'Field Worker',
    role: 'field_worker',
    locale: 'en',
  ),
);

void main() {
  late _MemSecureStorage storage;
  late TokenStore store;

  setUp(() {
    storage = _MemSecureStorage();
    store = TokenStore(storage: storage);
  });

  test('the default constructor builds a real secure storage backend', () {
    // Exercises the `?? const FlutterSecureStorage()` default branch — no
    // platform call is made, just construction.
    expect(TokenStore(), isA<TokenStore>());
  });

  test('save persists the pair and round-trips access/refresh/user', () async {
    await store.save(_pair(access: 'AAA', refresh: 'RRR'));

    expect(await store.accessToken, 'AAA');
    expect(await store.refreshToken, 'RRR');

    final user = await store.cachedUser;
    expect(user, isNotNull);
    expect(user!.email, 'field@example.com');
    expect(user.role, 'field_worker');
  });

  test('save stamps a future access expiry derived from expiresIn', () async {
    final before = DateTime.now().toUtc();
    await store.save(_pair(expiresIn: 1200));

    final expiry = await store.accessExpiry;
    expect(expiry, isNotNull);
    // ~20 minutes out (1200s), comfortably after the pre-save instant.
    expect(expiry!.isAfter(before), isTrue);
    expect(expiry.difference(before).inMinutes, greaterThanOrEqualTo(19));
  });

  test('accessExpiry is null before any save', () async {
    expect(await store.accessExpiry, isNull);
  });

  test(
    'updateTokens rotates access+refresh and bumps expiry, leaving user intact',
    () async {
      await store.save(_pair(access: 'old-a', refresh: 'old-r'));

      await store.updateTokens(
        accessToken: 'new-a',
        refreshToken: 'new-r',
        expiresIn: 600,
      );

      expect(await store.accessToken, 'new-a');
      expect(await store.refreshToken, 'new-r');
      // The cached user is untouched by a token rotation.
      expect((await store.cachedUser)!.id, 'u1');
    },
  );

  test('cachedUser is null when nothing is stored', () async {
    expect(await store.cachedUser, isNull);
  });

  test('cachedUser swallows malformed json and returns null', () async {
    // Seed a corrupt user blob directly at the wire key.
    storage.data['fb_user'] = 'this-is-not-json';
    expect(await store.cachedUser, isNull);
  });

  test('cachedUser decodes a well-formed stored blob', () async {
    storage.data['fb_user'] = jsonEncode({
      'id': 'u9',
      'org_id': 'org9',
      'email': 'a@b.com',
      'display_name': 'Nine',
      'role': 'superintendent',
      'locale': 'es',
    });
    final user = await store.cachedUser;
    expect(user!.id, 'u9');
    expect(user.role, 'superintendent');
    expect(user.locale, 'es');
  });

  test('hasSession reflects the presence of a refresh token', () async {
    expect(await store.hasSession, isFalse);
    await store.save(_pair());
    expect(await store.hasSession, isTrue);
  });

  test('clear wipes every stored key', () async {
    await store.save(_pair());
    expect(storage.data, isNotEmpty);

    await store.clear();

    expect(storage.data, isEmpty);
    expect(await store.accessToken, isNull);
    expect(await store.refreshToken, isNull);
    expect(await store.cachedUser, isNull);
    expect(await store.hasSession, isFalse);
  });
}
