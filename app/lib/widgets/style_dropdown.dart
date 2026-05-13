import 'package:flutter/material.dart';

/// 5 story style options used by Plan 9-A.
const List<String> kStyleOptions = [
  '温馨治愈',
  '冒险探索',
  '搞笑欢乐',
  '神奇魔法',
  '科普认知',
];

class StyleDropdown extends StatelessWidget {
  final String selected;
  final ValueChanged<String> onChanged;

  const StyleDropdown({
    super.key,
    required this.selected,
    required this.onChanged,
  });

  @override
  Widget build(BuildContext context) {
    return DropdownButtonFormField<String>(
      value: selected,
      decoration: const InputDecoration(
        labelText: '故事风格',
        border: OutlineInputBorder(),
      ),
      items: kStyleOptions
          .map((s) => DropdownMenuItem(value: s, child: Text(s)))
          .toList(),
      onChanged: (v) {
        if (v != null) onChanged(v);
      },
    );
  }
}
