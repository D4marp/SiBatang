import 'package:flutter/material.dart';

/// Operator desktop memakai sesi staf individual (OIDC Authorization Code + PKCE via browser sistem), BUKAN
/// kredensial perangkat. Alur OIDC belum tersedia di backend (GET /auth/staff/login -> 501), sehingga layar ini
/// sengaja belum memanggil endpoint antrean. Endpoint siap: ready/pause/call-next/recall/start/complete/no-show.
class OperatorScreen extends StatelessWidget {
  const OperatorScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return const Scaffold(
      body: Center(
        child: Padding(
          padding: EdgeInsets.all(32),
          child: Text('Mode operator menunggu integrasi OIDC staf (lihat docs/OIDC.md).\nPerangkat ini tidak membawa wewenang staf.',
              textAlign: TextAlign.center, style: TextStyle(fontSize: 24)),
        ),
      ),
    );
  }
}
