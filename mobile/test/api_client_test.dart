import 'dart:convert';
import 'dart:typed_data';

import 'package:buildos_field/services/api_client.dart';
import 'package:buildos_field/services/api_error.dart';
import 'package:buildos_field/services/token_store.dart';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';

/// A JSON [ResponseBody] with the content-type dio needs to decode to a Map.
ResponseBody _json(int status, Map<String, dynamic> body) =>
    ResponseBody.fromString(
      jsonEncode(body),
      status,
      headers: {
        Headers.contentTypeHeader: [Headers.jsonContentType],
      },
    );

/// Scripts dio at the transport seam: each fetch pops the next queued item —
/// a [ResponseBody] is returned, an [Exception] is thrown (transport failure).
/// An empty queue yields an empty 200 envelope.
class _FakeAdapter implements HttpClientAdapter {
  final List<Object> queue = [];
  final List<RequestOptions> seen = [];
  int calls = 0;

  @override
  Future<ResponseBody> fetch(
    RequestOptions options,
    Stream<Uint8List>? requestStream,
    Future<void>? cancelFuture,
  ) async {
    calls++;
    seen.add(options);
    final next = queue.isNotEmpty
        ? queue.removeAt(0)
        : _json(200, {'data': <String, dynamic>{}});
    if (next is Exception) throw next;
    return next as ResponseBody;
  }

  @override
  void close({bool force = false}) {}
}

/// In-memory [TokenStore] — overrides exactly the surface [ApiClient] touches
/// so no flutter_secure_storage platform channel is required. The default
/// super() builds a real (unused) FlutterSecureStorage that is never called.
class _MemTokenStore extends TokenStore {
  _MemTokenStore({this.access, this.refresh});

  String? access;
  String? refresh;
  int updateCount = 0;
  bool cleared = false;

  @override
  Future<String?> get accessToken async => access;

  @override
  Future<String?> get refreshToken async => refresh;

  @override
  Future<void> updateTokens({
    required String accessToken,
    required String refreshToken,
    required int expiresIn,
  }) async {
    updateCount++;
    access = accessToken;
    refresh = refreshToken;
  }

  @override
  Future<void> clear() async {
    cleared = true;
    access = null;
    refresh = null;
  }
}

ApiClient _client(_FakeAdapter main, _FakeAdapter refresh, TokenStore tokens) {
  final d = Dio(BaseOptions(baseUrl: 'http://test.local'))
    ..httpClientAdapter = main;
  final r = Dio(BaseOptions(baseUrl: 'http://test.local'))
    ..httpClientAdapter = refresh;
  return ApiClient(tokens: tokens, dio: d, refreshDio: r);
}

void main() {
  test('getJson unwraps the {data} envelope and attaches the bearer', () async {
    final main = _FakeAdapter()
      ..queue.add(
        _json(200, {
          'data': {'hello': 'world'},
        }),
      );
    final c = _client(
      main,
      _FakeAdapter(),
      _MemTokenStore(access: 'a', refresh: 'r'),
    );

    final out = await c.getJson('/x');

    expect(out['hello'], 'world');
    expect(main.seen.single.headers['Authorization'], 'Bearer a');
  });

  test('a non-2xx error envelope surfaces as a typed ApiError', () async {
    final main = _FakeAdapter()
      ..queue.add(
        _json(422, {
          'error': {'code': ErrorCode.validationError, 'message': 'bad input'},
        }),
      );
    final c = _client(
      main,
      _FakeAdapter(),
      _MemTokenStore(access: 'a', refresh: 'r'),
    );

    await expectLater(
      c.postJson('/x', body: <String, dynamic>{}),
      throwsA(
        isA<ApiError>()
            .having((e) => e.status, 'status', 422)
            .having((e) => e.code, 'code', ErrorCode.validationError)
            .having((e) => e.message, 'message', 'bad input'),
      ),
    );
  });

  test('a transport failure surfaces as a NETWORK_ERROR ApiError', () async {
    final main = _FakeAdapter()..queue.add(Exception('socket down'));
    final c = _client(
      main,
      _FakeAdapter(),
      _MemTokenStore(access: 'a', refresh: 'r'),
    );

    await expectLater(
      c.getJson('/x'),
      throwsA(
        isA<ApiError>()
            .having((e) => e.code, 'code', ErrorCode.networkError)
            .having((e) => e.status, 'status', 0),
      ),
    );
  });

  test('a 401 refreshes once then retries the original request', () async {
    final main = _FakeAdapter()
      ..queue.add(
        _json(401, {
          'error': {'code': ErrorCode.unauthorized, 'message': 'expired'},
        }),
      )
      ..queue.add(
        _json(200, {
          'data': {'ok': true},
        }),
      );
    // _doRefresh reads res.data directly (no envelope unwrap), so the token
    // fields sit at the top level of the refresh response.
    final refresh = _FakeAdapter()
      ..queue.add(
        _json(200, {
          'access_token': 'new',
          'refresh_token': 'r2',
          'expires_in': 900,
        }),
      );
    final tokens = _MemTokenStore(access: 'old', refresh: 'r1');
    final c = _client(main, refresh, tokens);

    final out = await c.getJson('/x');

    expect(out['ok'], true);
    expect(refresh.calls, 1); // exactly one refresh
    expect(main.calls, 2); // original + retry
    expect(tokens.access, 'new'); // rotated access token stored
    expect(tokens.refresh, 'r2'); // rotated refresh token stored
    expect(main.seen.last.headers['Authorization'], 'Bearer new');
  });

  test(
    'a 401 with no refresh token clears the session and fires onSessionExpired',
    () async {
      final main = _FakeAdapter()
        ..queue.add(
          _json(401, {
            'error': {'code': ErrorCode.unauthorized, 'message': 'expired'},
          }),
        );
      final tokens = _MemTokenStore(access: 'old', refresh: null);
      final c = _client(main, _FakeAdapter(), tokens);
      var expired = false;
      c.onSessionExpired = () => expired = true;

      await expectLater(c.getJson('/x'), throwsA(isA<ApiError>()));

      expect(expired, isTrue);
      expect(tokens.cleared, isTrue);
    },
  );

  test('a skipAuth request never refreshes and attaches no bearer', () async {
    final main = _FakeAdapter()
      ..queue.add(
        _json(401, {
          'error': {'code': ErrorCode.invalidCredentials, 'message': 'no'},
        }),
      );
    final refresh = _FakeAdapter();
    final c = _client(main, refresh, _MemTokenStore(access: 'a', refresh: 'r'));

    await expectLater(
      c.postJson(
        '/api/v1/auth/login',
        body: <String, dynamic>{},
        options: skipAuth(),
      ),
      throwsA(
        isA<ApiError>().having(
          (e) => e.code,
          'code',
          ErrorCode.invalidCredentials,
        ),
      ),
    );

    expect(refresh.calls, 0);
    expect(main.seen.single.headers.containsKey('Authorization'), isFalse);
  });

  test('a retried request that still fails surfaces the error', () async {
    final main = _FakeAdapter()
      ..queue.add(
        _json(401, {
          'error': {'code': ErrorCode.unauthorized, 'message': 'x'},
        }),
      )
      // The retry (post-refresh) itself fails with a 500.
      ..queue.add(
        _json(500, {
          'error': {'code': ErrorCode.internalError, 'message': 'boom'},
        }),
      );
    final refresh = _FakeAdapter()
      ..queue.add(
        _json(200, {
          'access_token': 'new',
          'refresh_token': 'r2',
          'expires_in': 900,
        }),
      );
    final c = _client(
      main,
      refresh,
      _MemTokenStore(access: 'old', refresh: 'r1'),
    );

    await expectLater(
      c.getJson('/x'),
      throwsA(
        isA<ApiError>()
            .having((e) => e.status, 'status', 500)
            .having((e) => e.code, 'code', ErrorCode.internalError),
      ),
    );
    expect(main.calls, 2); // original + the failing retry
    expect(refresh.calls, 1);
  });

  test('a refresh transport failure clears the session', () async {
    final main = _FakeAdapter()
      ..queue.add(
        _json(401, {
          'error': {'code': ErrorCode.unauthorized, 'message': 'x'},
        }),
      );
    // The refresh POST itself throws — _doRefresh swallows it as false.
    final refresh = _FakeAdapter()..queue.add(Exception('refresh offline'));
    final tokens = _MemTokenStore(access: 'old', refresh: 'r1');
    final c = _client(main, refresh, tokens);
    var expired = false;
    c.onSessionExpired = () => expired = true;

    await expectLater(c.getJson('/x'), throwsA(isA<ApiError>()));

    expect(expired, isTrue);
    expect(tokens.cleared, isTrue);
  });

  test('a non-envelope error body maps to an UNKNOWN ApiError', () async {
    // A non-2xx whose body has no { error } envelope and a real status.
    final main = _FakeAdapter()..queue.add(_json(500, <String, dynamic>{}));
    final c = _client(
      main,
      _FakeAdapter(),
      _MemTokenStore(access: 'a', refresh: 'r'),
    );

    await expectLater(
      c.getJson('/x'),
      throwsA(
        isA<ApiError>()
            .having((e) => e.code, 'code', ErrorCode.unknown)
            .having((e) => e.status, 'status', 500),
      ),
    );
  });

  test('concurrent 401s share a single in-flight refresh', () async {
    final main = _FakeAdapter()
      ..queue.add(
        _json(401, {
          'error': {'code': ErrorCode.unauthorized, 'message': 'x'},
        }),
      )
      ..queue.add(
        _json(401, {
          'error': {'code': ErrorCode.unauthorized, 'message': 'x'},
        }),
      )
      ..queue.add(
        _json(200, {
          'data': {'n': 1},
        }),
      )
      ..queue.add(
        _json(200, {
          'data': {'n': 2},
        }),
      );
    final refresh = _FakeAdapter()
      ..queue.add(
        _json(200, {
          'access_token': 'new',
          'refresh_token': 'r2',
          'expires_in': 900,
        }),
      );
    final tokens = _MemTokenStore(access: 'old', refresh: 'r1');
    final c = _client(main, refresh, tokens);

    final results = await Future.wait([c.getJson('/a'), c.getJson('/b')]);

    expect(refresh.calls, 1); // single-flight: both 401s shared one refresh
    expect(results.length, 2);
    expect(tokens.updateCount, 1);
  });
}
