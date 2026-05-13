import 'package:flutter/material.dart';

/// Single-select chip row for story duration (5 / 10 / 15 minutes).
class DurationChips extends StatelessWidget {
  final int selected;
  final ValueChanged<int> onChanged;

  const DurationChips({
    super.key,
    required this.selected,
    required this.onChanged,
  });

  static const List<int> options = [5, 10, 15];

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: 12,
      children: options.map((d) {
        return ChoiceChip(
          label: Text('$d 分钟'),
          selected: selected == d,
          onSelected: (_) => onChanged(d),
        );
      }).toList(),
    );
  }
}
