import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

class HomeScreen extends StatelessWidget {
  const HomeScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('SIBATANG')),
      body: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Antrean Polres Batang', style: Theme.of(context).textTheme.headlineSmall),
            const SizedBox(height: 8),
            const Text('Daftar online, scan QR di kiosk saat tiba, lalu tunggu panggilan di loket.'),
            const SizedBox(height: 24),
            FilledButton(onPressed: () => context.go('/booking'), child: const Text('Daftar antrean')),
          ],
        ),
      ),
    );
  }
}
