import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../state/auth_state.dart';

/// Plan 9-A login screen.
///
/// Two-step phone + SMS code flow:
/// 1. user types 11-digit phone -> tap "发送验证码" -> backend sends SMS
///    (dev environment always returns 123456).
/// 2. user types 6-digit code -> tap "登录 / 注册" -> first-time users are
///    auto-registered; AuthNotifier transitions to [AuthAuthenticated] and the
///    router redirect (Task 10) handles navigation.
class LoginScreen extends ConsumerStatefulWidget {
  const LoginScreen({super.key});

  @override
  ConsumerState<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends ConsumerState<LoginScreen> {
  final _phone = TextEditingController();
  final _code = TextEditingController();
  Timer? _countdownTimer;
  int _countdown = 0;
  bool _sending = false;
  bool _logging = false;

  @override
  void initState() {
    super.initState();
    _phone.addListener(_refresh);
    _code.addListener(_refresh);
  }

  @override
  void dispose() {
    _countdownTimer?.cancel();
    _phone.dispose();
    _code.dispose();
    super.dispose();
  }

  void _refresh() => setState(() {});

  void _startCountdown() {
    setState(() => _countdown = 60);
    _countdownTimer?.cancel();
    _countdownTimer = Timer.periodic(const Duration(seconds: 1), (t) {
      if (!mounted) {
        t.cancel();
        return;
      }
      setState(() => _countdown--);
      if (_countdown <= 0) t.cancel();
    });
  }

  void _toast(String msg) {
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(content: Text(msg)));
  }

  Future<void> _sendCode() async {
    final phone = _phone.text.trim();
    if (phone.length != 11) {
      _toast('请输入 11 位手机号');
      return;
    }
    setState(() => _sending = true);
    try {
      await ref.read(authProvider.notifier).sendSmsCode(phone);
      if (!mounted) return;
      _toast('验证码已发送');
      _startCountdown();
    } catch (e) {
      if (mounted) _toast(_friendly(e));
    } finally {
      if (mounted) setState(() => _sending = false);
    }
  }

  Future<void> _login() async {
    final phone = _phone.text.trim();
    final code = _code.text.trim();
    if (phone.length != 11 || code.length != 6) return;
    setState(() => _logging = true);
    try {
      await ref
          .read(authProvider.notifier)
          .loginOrRegister(phone: phone, code: code);
      // Router redirect (Task 10) handles navigation on auth state change.
    } catch (e) {
      if (mounted) _toast(_friendly(e));
    } finally {
      if (mounted) setState(() => _logging = false);
    }
  }

  String _friendly(Object e) {
    final s = e.toString();
    return s.startsWith('Exception: ') ? s.substring('Exception: '.length) : s;
  }

  @override
  Widget build(BuildContext context) {
    final canSend = _countdown == 0 && !_sending && _phone.text.length == 11;
    final canLogin = !_logging &&
        _phone.text.length == 11 &&
        _code.text.length == 6;

    return Scaffold(
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const SizedBox(height: 32),
              const Text(
                '🐼',
                style: TextStyle(fontSize: 96),
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: 12),
              Text(
                '爱宝',
                style: Theme.of(context).textTheme.headlineLarge,
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: 4),
              const Text(
                '为孩子讲温柔的故事',
                textAlign: TextAlign.center,
                style: TextStyle(fontSize: 16, color: Colors.black54),
              ),
              const SizedBox(height: 48),
              TextField(
                controller: _phone,
                keyboardType: TextInputType.phone,
                inputFormatters: [
                  FilteringTextInputFormatter.digitsOnly,
                  LengthLimitingTextInputFormatter(11),
                ],
                decoration: const InputDecoration(
                  labelText: '手机号',
                  border: OutlineInputBorder(),
                  prefixIcon: Icon(Icons.phone_android),
                ),
              ),
              const SizedBox(height: 16),
              Row(
                children: [
                  Expanded(
                    child: TextField(
                      controller: _code,
                      keyboardType: TextInputType.number,
                      inputFormatters: [
                        FilteringTextInputFormatter.digitsOnly,
                        LengthLimitingTextInputFormatter(6),
                      ],
                      decoration: const InputDecoration(
                        labelText: '验证码',
                        border: OutlineInputBorder(),
                        prefixIcon: Icon(Icons.sms),
                      ),
                    ),
                  ),
                  const SizedBox(width: 12),
                  SizedBox(
                    height: 56,
                    child: FilledButton.tonal(
                      onPressed: canSend ? _sendCode : null,
                      child: Text(
                        _sending
                            ? '发送中...'
                            : _countdown > 0
                                ? '${_countdown}s'
                                : '发送验证码',
                      ),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 32),
              SizedBox(
                height: 52,
                child: FilledButton(
                  onPressed: canLogin ? _login : null,
                  child: Text(_logging ? '登录中...' : '登录 / 注册'),
                ),
              ),
              const SizedBox(height: 24),
              const Text(
                '新用户首次登录会自动注册',
                textAlign: TextAlign.center,
                style: TextStyle(fontSize: 13, color: Colors.black54),
              ),
              const SizedBox(height: 4),
              const Text(
                '测试验证码: 123456',
                textAlign: TextAlign.center,
                style: TextStyle(fontSize: 13, color: Colors.black45),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
