import 'dart:convert';

class Child {
  final int id;
  final int userId;
  final String nickname;
  final String gender;
  final String birthday;
  final Map<String, dynamic>? profile;

  Child({
    required this.id,
    required this.userId,
    required this.nickname,
    required this.gender,
    required this.birthday,
    this.profile,
  });

  factory Child.fromJson(Map<String, dynamic> j) => Child(
        id: j['id'] as int,
        userId: j['user_id'] as int,
        nickname: j['nickname'] as String? ?? '',
        gender: j['gender'] as String? ?? '',
        birthday: j['birthday'] as String? ?? '',
        profile: _parseProfile(j['profile']),
      );

  /// Backend currently serialises `profile` as a JSON-encoded string
  /// (`string(c.Profile)`), but a future refactor may emit it as a real Map.
  /// Accept both shapes plus null / empty-object sentinel.
  static Map<String, dynamic>? _parseProfile(dynamic raw) {
    if (raw == null) return null;
    if (raw is Map<String, dynamic>) return raw;
    if (raw is String) {
      if (raw.isEmpty || raw == '{}') return null;
      try {
        final decoded = jsonDecode(raw);
        if (decoded is Map<String, dynamic>) return decoded;
      } catch (_) {}
    }
    return null;
  }
}
