import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/models/heartbeat.dart';
import '../main.dart' show apiClientProvider;

/// FutureProvider.family keyed by childId — re-fetches when childId changes.
/// Server logic (Plan 8): time-of-day greeting + up to 5 active storylines.
final heartbeatProvider =
    FutureProvider.family<HeartbeatResponse, int>((ref, childId) async {
  final api = ref.watch(apiClientProvider);
  return api.getHeartbeat(childId);
});
