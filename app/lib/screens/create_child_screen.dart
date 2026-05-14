import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../state/child_state.dart';

/// Onboarding form for users who have logged in but have not yet created a
/// child profile. Once the child is created, the router redirect re-evaluates
/// and pushes the user on to the home screen.
///
/// Back navigation is disabled — until a child exists, the rest of the app is
/// not meaningful.
class CreateChildScreen extends ConsumerStatefulWidget {
  const CreateChildScreen({super.key});

  @override
  ConsumerState<CreateChildScreen> createState() => _CreateChildScreenState();
}

class _CreateChildScreenState extends ConsumerState<CreateChildScreen> {
  final _formKey = GlobalKey<FormState>();
  final _nicknameCtl = TextEditingController();
  String _gender = 'boy';
  late DateTime _birthday;
  bool _submitting = false;

  @override
  void initState() {
    super.initState();
    final now = DateTime.now();
    // Default to 3 years ago.
    _birthday = DateTime(now.year - 3, now.month, now.day);
  }

  @override
  void dispose() {
    _nicknameCtl.dispose();
    super.dispose();
  }

  String _fmtDate(DateTime d) =>
      '${d.year}-${d.month.toString().padLeft(2, '0')}-${d.day.toString().padLeft(2, '0')}';

  String _displayDate(DateTime d) => '${d.year} 年 ${d.month} 月 ${d.day} 日';

  Future<void> _pickBirthday() async {
    final now = DateTime.now();
    final first = DateTime(now.year - 15, now.month, now.day);
    final last = now;
    final picked = await showDatePicker(
      context: context,
      initialDate: _birthday,
      firstDate: first,
      lastDate: last,
      helpText: '选择孩子生日',
      cancelText: '取消',
      confirmText: '确定',
    );
    if (picked != null && mounted) {
      setState(() => _birthday = picked);
    }
  }

  void _toast(String msg) {
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(content: Text(msg)));
  }

  Future<void> _submit() async {
    if (!_formKey.currentState!.validate()) return;
    setState(() => _submitting = true);
    try {
      await ref.read(childProvider.notifier).createChild(
            nickname: _nicknameCtl.text.trim(),
            gender: _gender,
            birthday: _fmtDate(_birthday),
          );
      // Router redirect handles navigation once childProvider has data.
    } catch (e) {
      if (mounted) {
        final s = e.toString();
        _toast(s.startsWith('Exception: ') ? s.substring(11) : s);
      }
    } finally {
      if (mounted) setState(() => _submitting = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return PopScope(
      canPop: false,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('创建孩子档案'),
          automaticallyImplyLeading: false,
        ),
        body: SafeArea(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: Form(
              key: _formKey,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  const SizedBox(height: 8),
                  const Text(
                    '告诉爱宝一些关于孩子的事～',
                    style: TextStyle(fontSize: 15, color: Colors.black54),
                  ),
                  const SizedBox(height: 24),
                  TextFormField(
                    controller: _nicknameCtl,
                    maxLength: 30,
                    decoration: const InputDecoration(
                      labelText: '昵称',
                      border: OutlineInputBorder(),
                      prefixIcon: Icon(Icons.child_care),
                    ),
                    validator: (v) {
                      final s = (v ?? '').trim();
                      if (s.isEmpty) return '请输入昵称';
                      if (s.length > 30) return '昵称最长 30 个字';
                      return null;
                    },
                  ),
                  const SizedBox(height: 8),
                  const Text('性别',
                      style:
                          TextStyle(fontSize: 14, fontWeight: FontWeight.w600)),
                  Row(
                    children: [
                      Expanded(
                        child: RadioListTile<String>(
                          contentPadding: EdgeInsets.zero,
                          title: const Text('男孩'),
                          value: 'boy',
                          groupValue: _gender,
                          onChanged: (v) => setState(() => _gender = v!),
                        ),
                      ),
                      Expanded(
                        child: RadioListTile<String>(
                          contentPadding: EdgeInsets.zero,
                          title: const Text('女孩'),
                          value: 'girl',
                          groupValue: _gender,
                          onChanged: (v) => setState(() => _gender = v!),
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 8),
                  InkWell(
                    onTap: _pickBirthday,
                    borderRadius: BorderRadius.circular(8),
                    child: InputDecorator(
                      decoration: const InputDecoration(
                        labelText: '生日',
                        border: OutlineInputBorder(),
                        prefixIcon: Icon(Icons.cake_outlined),
                      ),
                      child: Text(_displayDate(_birthday)),
                    ),
                  ),
                  const SizedBox(height: 32),
                  SizedBox(
                    height: 52,
                    child: FilledButton(
                      onPressed: _submitting ? null : _submit,
                      child: Text(_submitting ? '保存中...' : '创建'),
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
