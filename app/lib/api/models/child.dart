class Child {
  final int id;
  final int userId;
  final String nickname;
  final String gender;
  final String birthday;
  Child({
    required this.id,
    required this.userId,
    required this.nickname,
    required this.gender,
    required this.birthday,
  });
  factory Child.fromJson(Map<String, dynamic> j) => Child(
        id: j['id'] as int,
        userId: j['user_id'] as int,
        nickname: j['nickname'] as String? ?? '',
        gender: j['gender'] as String? ?? '',
        birthday: j['birthday'] as String? ?? '',
      );
}
