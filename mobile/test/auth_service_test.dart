import 'package:buildos_field/services/api_client.dart';
import 'package:buildos_field/services/auth_service.dart';
import 'package:buildos_field/services/token_store.dart';
import 'package:dio/dio.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';

/// In-memory [FlutterSecureStorage] so the real [TokenStore] persists without
/// a platform channel — overrides only the read/write/deleteAll surface.
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

/// Scripts [ApiClient.postJson] at the seam: the next queued item is returned
/// (a Map) or thrown (an Exception). Records the path + body of each call so
/// tests can assert the login/logout wire contract.
class _FakeApi extends ApiClient {
  _FakeApi() : super(tokens: TokenStore(storage: _MemSecureStorage()));

  final List<String> postPaths = [];
  Object? lastBody;
  Object? nextResponse;
  int postCount = 0;

  @override
  Future<Map<String, dynamic>> postJson(
    String path, {
    Object? body,
    Options? options,
  }) async {
    postCount++;
    postPaths.add(path);
    lastBody = body;
    final r = nextResponse;
    if (r is Exception) throw r;
    return (r as Map?)?.cast<String, dynamic>() ?? <String, dynamic>{};
  }
}

Map<String, dynamic> _loginEnvelope() => {
  'access_token': 'access-123',
  'refresh_token': 'refresh-456',
  'expires_in': 900,
  'user': {
    'id': 'u1',
    'org_id': 'org1',
    'email': 'field@example.com',
    'display_name': 'Field Worker',
    'role': 'field_worker',
    'locale': 'en',
  },
};

void main() {
  late _MemSecureStorage storage;
  late TokenStore tokens;
  late _FakeApi api;
  late AuthService auth;

  setUp(() {
    storage = _MemSecureStorage();
    tokens = TokenStore(storage: storage);
    api = _FakeApi();
    auth = AuthService(api: api, tokens: tokens);
  });

  test(
    'login posts credentials, persists the pair, and returns the user',
    () async {
      api.nextResponse = _loginEnvelope();

      final user = await auth.login('field@example.com', 'hunter2');

      expect(user.email, 'field@example.com');
      expect(api.postPaths.single, '/api/v1/auth/login');
      final body = api.lastBody as Map<String, dynamic>;
      expect(body['email'], 'field@example.com');
      expect(body['password'], 'hunter2');
      // The token pair landed in the store.
      expect(await tokens.accessToken, 'access-123');
      expect(await tokens.refreshToken, 'refresh-456');
      expect((await tokens.cachedUser)!.id, 'u1');
    },
  );

  test('logout revokes server-side then clears the local store', () async {
    api.nextResponse = _loginEnvelope();
    await auth.login('field@example.com', 'hunter2');
    expect(await tokens.hasSession, isTrue);

    api.nextResponse = <String, dynamic>{}; // logout 200
    await auth.logout();

    // The revoke carried the refresh token...
    expect(api.postPaths.last, '/api/v1/auth/logout');
    final body = api.lastBody as Map<String, dynamic>;
    expect(body['refresh_token'], 'refresh-456');
    // ...and the local session is gone.
    expect(await tokens.hasSession, isFalse);
    expect(storage.data, isEmpty);
  });

  test(
    'logout with no refresh token skips the server call but still clears',
    () async {
      // No prior login: refreshToken is null.
      await auth.logout();

      expect(api.postCount, 0); // server never hit
      expect(await tokens.hasSession, isFalse);
    },
  );

  test('logout swallows a server error and clears locally anyway', () async {
    api.nextResponse = _loginEnvelope();
    await auth.login('field@example.com', 'hunter2');

    // The revoke POST throws (offline / already-revoked).
    api.nextResponse = Exception('network down');
    await auth.logout();

    // The throw was caught; the local clear still happened.
    expect(await tokens.hasSession, isFalse);
    expect(storage.data, isEmpty);
  });

  test('cachedUser surfaces the stored user for an offline launch', () async {
    api.nextResponse = _loginEnvelope();
    await auth.login('field@example.com', 'hunter2');

    final cached = await auth.cachedUser();
    expect(cached!.email, 'field@example.com');
  });

  test('hasSession is false cold and true after a login', () async {
    expect(await auth.hasSession(), isFalse);

    api.nextResponse = _loginEnvelope();
    await auth.login('field@example.com', 'hunter2');

    expect(await auth.hasSession(), isTrue);
  });
}
