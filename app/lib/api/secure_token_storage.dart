import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'token_storage.dart';

class SecureTokenStorage implements TokenStorage {
  static const _key = 'access_token';
  final FlutterSecureStorage _storage;
  SecureTokenStorage({FlutterSecureStorage? storage})
      : _storage = storage ?? const FlutterSecureStorage();

  @override
  Future<String?> read() => _storage.read(key: _key);

  @override
  Future<void> write(String value) =>
      _storage.write(key: _key, value: value);

  @override
  Future<void> delete() => _storage.delete(key: _key);
}
