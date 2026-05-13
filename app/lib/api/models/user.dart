class User {
  final int id;
  final String nickname;
  final String subscriptionTier;
  User({required this.id, required this.nickname, required this.subscriptionTier});
  factory User.fromJson(Map<String, dynamic> j) => User(
        id: j['id'] as int,
        nickname: j['nickname'] as String? ?? '',
        subscriptionTier: j['subscription_tier'] as String? ?? 'free',
      );
}
