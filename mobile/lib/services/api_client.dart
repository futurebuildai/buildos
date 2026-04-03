import 'dart:convert';
import 'package:http/http.dart' as http;

/// Result of an API call.
class ApiResponse {
  final int statusCode;
  final Map<String, dynamic>? body;
  final String? error;

  const ApiResponse({
    required this.statusCode,
    this.body,
    this.error,
  });

  bool get isSuccess => statusCode >= 200 && statusCode < 300;
  bool get isDuplicate => statusCode == 409;
  bool get isServerError => statusCode >= 500;
  bool get isClientError => statusCode >= 400 && statusCode < 500;
}

/// Response from the sync endpoint containing tasks and metadata.
class SyncResponse {
  final List<Map<String, dynamic>> tasks;
  final String serverTimestamp;

  const SyncResponse({
    required this.tasks,
    required this.serverTimestamp,
  });

  factory SyncResponse.fromJson(Map<String, dynamic> json) {
    final tasksRaw = json['tasks'] as List<dynamic>? ?? [];
    return SyncResponse(
      tasks: tasksRaw.cast<Map<String, dynamic>>(),
      serverTimestamp: json['server_timestamp'] as String? ?? '',
    );
  }
}

/// HTTP client wrapper for the FutureBuild OS backend.
///
/// Base URL defaults to 10.0.2.2:8080 (Android emulator localhost).
/// All requests include JWT Authorization header when available.
class ApiClient {
  final String baseUrl;
  String? _token;
  final http.Client _client;

  ApiClient({
    this.baseUrl = 'http://10.0.2.2:8080',
    http.Client? client,
  }) : _client = client ?? http.Client();

  /// Set the JWT auth token.
  void setToken(String? token) {
    _token = token;
  }

  Map<String, String> get _headers {
    final headers = <String, String>{
      'Content-Type': 'application/json',
      'Accept': 'application/json',
    };
    if (_token != null && _token!.isNotEmpty) {
      headers['Authorization'] = 'Bearer $_token';
    }
    return headers;
  }

  /// Pull sync: GET /api/v1/field/sync?since={since}
  ///
  /// Returns tasks updated since the given timestamp.
  Future<ApiResponse> syncField({String? since}) async {
    try {
      final uri = Uri.parse('$baseUrl/api/v1/field/sync').replace(
        queryParameters:
            since != null && since.isNotEmpty ? {'since': since} : null,
      );
      final response = await _client.get(uri, headers: _headers);
      return _parseResponse(response);
    } catch (e) {
      return ApiResponse(statusCode: 0, error: e.toString());
    }
  }

  /// Report task progress: POST /api/v1/field/progress
  Future<ApiResponse> reportProgress(Map<String, dynamic> body) async {
    return _post('/api/v1/field/progress', body);
  }

  /// Crew check-in: POST /api/v1/field/checkin
  Future<ApiResponse> checkin(Map<String, dynamic> body) async {
    return _post('/api/v1/field/checkin', body);
  }

  /// Submit daily log: POST /api/v1/field/daily-log
  Future<ApiResponse> submitDailyLog(Map<String, dynamic> body) async {
    return _post('/api/v1/field/daily-log', body);
  }

  Future<ApiResponse> _post(
      String path, Map<String, dynamic> body) async {
    try {
      final uri = Uri.parse('$baseUrl$path');
      final response = await _client.post(
        uri,
        headers: _headers,
        body: jsonEncode(body),
      );
      return _parseResponse(response);
    } catch (e) {
      return ApiResponse(statusCode: 0, error: e.toString());
    }
  }

  ApiResponse _parseResponse(http.Response response) {
    Map<String, dynamic>? body;
    try {
      if (response.body.isNotEmpty) {
        body = jsonDecode(response.body) as Map<String, dynamic>;
      }
    } catch (_) {
      // Body is not JSON; that is acceptable for some responses.
    }
    return ApiResponse(
      statusCode: response.statusCode,
      body: body,
      error: response.statusCode >= 400 ? response.body : null,
    );
  }

  /// Dispose the underlying HTTP client.
  void dispose() {
    _client.close();
  }
}
