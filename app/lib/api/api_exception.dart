class ApiException implements Exception {
  final int statusCode;
  final String reason;
  final String userMsg;
  final dynamic raw;

  ApiException({
    required this.statusCode,
    required this.reason,
    required this.userMsg,
    this.raw,
  });

  factory ApiException.fromResponse(int status, Map<String, dynamic> body) {
    return ApiException(
      statusCode: status,
      reason: body['reason']?.toString() ?? 'unknown',
      userMsg: body['user_msg']?.toString() ?? '请求失败',
      raw: body,
    );
  }

  @override
  String toString() => 'ApiException($statusCode $reason): $userMsg';
}
