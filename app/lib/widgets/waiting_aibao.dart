import 'package:flutter/material.dart';

/// Full-screen translucent overlay shown while the story is being generated.
///
/// Use it via [showWaitingAibao], which displays the overlay in a dialog while
/// awaiting [task] and returns its result (or null if the task threw — the
/// caller is responsible for surfacing errors).
class WaitingAibao extends StatelessWidget {
  const WaitingAibao({super.key});

  @override
  Widget build(BuildContext context) {
    return Container(
      color: Colors.black54,
      alignment: Alignment.center,
      child: Card(
        margin: const EdgeInsets.symmetric(horizontal: 40),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 32, vertical: 28),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: const [
              Text('🐼', style: TextStyle(fontSize: 80)),
              SizedBox(height: 16),
              Text(
                '爱宝在想故事...',
                style: TextStyle(fontSize: 18, fontWeight: FontWeight.w500),
              ),
              SizedBox(height: 20),
              CircularProgressIndicator(),
            ],
          ),
        ),
      ),
    );
  }
}

/// Wraps [task] in a modal "waiting" dialog. Returns the task's value, or
/// null if the task threw. Always closes the dialog before returning.
Future<T?> showWaitingAibao<T>(
  BuildContext context,
  Future<T> task,
) async {
  showDialog<void>(
    context: context,
    barrierDismissible: false,
    builder: (_) => const PopScope(
      canPop: false,
      child: WaitingAibao(),
    ),
  );
  try {
    final result = await task;
    if (context.mounted) Navigator.of(context, rootNavigator: true).pop();
    return result;
  } catch (_) {
    if (context.mounted) Navigator.of(context, rootNavigator: true).pop();
    rethrow;
  }
}
