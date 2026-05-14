import 'package:flutter/material.dart';
import '../api/models/bootstrap.dart';

/// Renders a single BOOTSTRAP question inside a Card. The widget is
/// stateless — the parent screen owns the draft map and pipes [value] +
/// [onChanged] in.
///
/// `value` is `dynamic` because the answer type depends on `question.type`:
///   - text          -> `String`
///   - single_select -> `String`
///   - multi_select  -> `List<String>`
///   - boolean       -> `bool`
class BootstrapQuestionCard extends StatelessWidget {
  final BootstrapQuestion question;
  final dynamic value;
  final void Function(dynamic) onChanged;

  const BootstrapQuestionCard({
    super.key,
    required this.question,
    required this.value,
    required this.onChanged,
  });

  @override
  Widget build(BuildContext context) {
    final hintStyle = Theme.of(context)
        .textTheme
        .bodySmall
        ?.copyWith(color: Theme.of(context).hintColor);

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              question.label + (question.required ? ' *' : ''),
              style: const TextStyle(fontSize: 16, fontWeight: FontWeight.bold),
            ),
            if (question.hint != null && question.hint!.isNotEmpty) ...[
              const SizedBox(height: 4),
              Text(question.hint!, style: hintStyle),
            ],
            const SizedBox(height: 12),
            _buildInput(context),
          ],
        ),
      ),
    );
  }

  Widget _buildInput(BuildContext context) {
    switch (question.type) {
      case 'text':
        return _TextInput(
          initial: (value as String?) ?? '',
          maxLength: question.maxLength,
          onChanged: onChanged,
        );
      case 'single_select':
        return _SingleSelect(
          options: question.options ?? const [],
          selected: (value as String?) ?? '',
          onChanged: onChanged,
        );
      case 'multi_select':
        final raw = value;
        final selected = raw is List
            ? raw.cast<String>().toList()
            : <String>[];
        return _MultiSelect(
          options: question.options ?? const [],
          selected: selected,
          onChanged: onChanged,
        );
      case 'boolean':
        return SwitchListTile(
          contentPadding: EdgeInsets.zero,
          title: const Text('是'),
          value: (value as bool?) ?? false,
          onChanged: onChanged,
        );
      default:
        return Text('暂不支持的问题类型: ${question.type}');
    }
  }
}

class _TextInput extends StatefulWidget {
  final String initial;
  final int? maxLength;
  final void Function(String) onChanged;
  const _TextInput({
    required this.initial,
    required this.maxLength,
    required this.onChanged,
  });

  @override
  State<_TextInput> createState() => _TextInputState();
}

class _TextInputState extends State<_TextInput> {
  late final TextEditingController _ctl;

  @override
  void initState() {
    super.initState();
    _ctl = TextEditingController(text: widget.initial);
  }

  @override
  void dispose() {
    _ctl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: _ctl,
      maxLength: widget.maxLength,
      maxLines: null,
      decoration: const InputDecoration(
        border: OutlineInputBorder(),
        isDense: true,
      ),
      onChanged: widget.onChanged,
    );
  }
}

class _SingleSelect extends StatelessWidget {
  final List<String> options;
  final String selected;
  final void Function(String) onChanged;
  const _SingleSelect({
    required this.options,
    required this.selected,
    required this.onChanged,
  });

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: 8,
      runSpacing: 4,
      children: [
        for (final o in options)
          ChoiceChip(
            label: Text(o),
            selected: selected == o,
            onSelected: (v) {
              if (v) onChanged(o);
            },
          ),
      ],
    );
  }
}

class _MultiSelect extends StatelessWidget {
  final List<String> options;
  final List<String> selected;
  final void Function(List<String>) onChanged;
  const _MultiSelect({
    required this.options,
    required this.selected,
    required this.onChanged,
  });

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: 8,
      runSpacing: 4,
      children: [
        for (final o in options)
          ChoiceChip(
            label: Text(o),
            selected: selected.contains(o),
            onSelected: (v) {
              final next = List<String>.from(selected);
              if (v) {
                if (!next.contains(o)) next.add(o);
              } else {
                next.remove(o);
              }
              onChanged(next);
            },
          ),
      ],
    );
  }
}
