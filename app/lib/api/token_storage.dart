/// Thin abstraction over secure token persistence so tests can substitute
/// an in-memory implementation without pulling in plugin native code.
/// The Flutter-plugin-backed implementation lives in
/// `secure_token_storage.dart` and is imported only by app entrypoints.
abstract class TokenStorage {
  Future<String?> read();
  Future<void> write(String value);
  Future<void> delete();
}

class InMemoryTokenStorage implements TokenStorage {
  String? _value;
  @override
  Future<String?> read() async => _value;
  @override
  Future<void> write(String value) async => _value = value;
  @override
  Future<void> delete() async => _value = null;
}
