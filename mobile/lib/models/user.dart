/// Mirrors internal/models.User (PasswordHash / OIDCSubject are json:"-").
class User {
  const User({
    required this.id,
    required this.orgId,
    required this.email,
    required this.displayName,
    required this.role,
    required this.locale,
  });

  final String id;
  final String orgId;
  final String email;
  final String displayName;
  final String role;
  final String locale;

  factory User.fromJson(Map<String, dynamic> json) => User(
    id: json['id'] as String? ?? '',
    orgId: json['org_id'] as String? ?? '',
    email: json['email'] as String? ?? '',
    displayName: json['display_name'] as String? ?? '',
    role: json['role'] as String? ?? 'field_worker',
    locale: json['locale'] as String? ?? 'en',
  );

  Map<String, dynamic> toJson() => {
    'id': id,
    'org_id': orgId,
    'email': email,
    'display_name': displayName,
    'role': role,
    'locale': locale,
  };
}

/// Success body of claim/login/refresh (internal/api/auth.go tokenPairResponse).
class TokenPair {
  const TokenPair({
    required this.accessToken,
    required this.refreshToken,
    required this.expiresIn,
    required this.user,
  });

  final String accessToken;
  final String refreshToken;

  /// Access-token lifetime in seconds (drives proactive refresh).
  final int expiresIn;
  final User user;

  factory TokenPair.fromJson(Map<String, dynamic> json) => TokenPair(
    accessToken: json['access_token'] as String? ?? '',
    refreshToken: json['refresh_token'] as String? ?? '',
    expiresIn: (json['expires_in'] as num?)?.toInt() ?? 0,
    user: User.fromJson(
      (json['user'] as Map?)?.cast<String, dynamic>() ?? const {},
    ),
  );
}
