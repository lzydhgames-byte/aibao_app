# Tests

## Running

```
flutter test
```

## Known local issue (Windows, no Visual Studio)

`flutter_tester.exe` on Windows requires the MSVC Visual C++ runtime
(installed by Visual Studio "Desktop development with C++" workload).
Without it, every test exits with:

```
Failed to load "...": Connection closed before test suite loaded.
```

This is an environmental limitation — the test source compiles cleanly
(`flutter analyze` is green) and will pass on:

- CI (Ubuntu runners)
- macOS / Linux dev boxes
- Windows boxes with Visual Studio Build Tools installed

To fix locally on Windows install
"Visual Studio Build Tools" → "Desktop development with C++" workload.

## Layout

- `api_client_test.dart` — mocks dio with `http_mock_adapter`, verifies
  sendSmsCode / loginOrRegister / getAudioUrl (3-state response). Uses
  `InMemoryTokenStorage` so no native plugin code is loaded.
- `widget_test.dart` — smoke test for placeholder home screen.
