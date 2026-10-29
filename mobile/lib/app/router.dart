import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../features/booking/booking_detail_screen.dart';
import '../features/booking/booking_flow_screen.dart';
import '../features/booking/home_screen.dart';

final routerProvider = Provider<GoRouter>((ref) {
  return GoRouter(
    routes: [
      GoRoute(path: '/', builder: (_, __) => const HomeScreen()),
      GoRoute(path: '/booking', builder: (_, __) => const BookingFlowScreen()),
      GoRoute(
        path: '/booking/:reference',
        builder: (_, state) =>
            BookingDetailScreen(reference: state.pathParameters['reference']!),
      ),
    ],
  );
});
