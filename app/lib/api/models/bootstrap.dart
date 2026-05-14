/// One question definition from `GET /api/v1/bootstrap/questions`.
///
/// Question types map to backend constants:
///   - "text"          -> TextField (respect maxLength)
///   - "single_select" -> Dropdown / ChoiceChip single-pick from options
///   - "multi_select"  -> ChoiceChip wrap (multi-pick from options)
///   - "boolean"       -> Switch
class BootstrapQuestion {
  final String id;
  final String label;
  final String type;
  final bool required;
  final int? maxLength;       // text only
  final List<String>? options; // single_select / multi_select only
  final String? hint;          // optional UI hint

  BootstrapQuestion({
    required this.id,
    required this.label,
    required this.type,
    required this.required,
    this.maxLength,
    this.options,
    this.hint,
  });

  factory BootstrapQuestion.fromJson(Map<String, dynamic> j) {
    final opts = j['options'];
    return BootstrapQuestion(
      id: j['id'] as String,
      label: j['label'] as String? ?? '',
      type: j['type'] as String? ?? 'text',
      required: j['required'] as bool? ?? false,
      maxLength: j['max_length'] as int?,
      options: opts == null
          ? null
          : (opts as List).cast<String>(),
      hint: j['hint'] as String?,
    );
  }

  /// True if the user can leave the answer blank (skip the form item).
  bool get optional => !required;
}

/// User-submitted answer to one question, paired with the q_id.
///
/// `value` is intentionally dynamic to support all question types:
///   - text         -> String
///   - single_select-> String (one option)
///   - multi_select -> `List<String>`
///   - boolean      -> bool
class BootstrapAnswer {
  final String qId;
  final dynamic value;

  BootstrapAnswer({required this.qId, required this.value});

  Map<String, dynamic> toJson() => {'q_id': qId, 'value': value};
}
