import '../../core/models.dart';

/// Template id-ID-v1: HARUS identik dengan backend/internal/domain/voice.go (Utterance). Teks bebas operator tidak pernah diputar.
String renderUtterance(Speech s) {
  final b = StringBuffer('Nomor antrean ${s.serviceSpoken}, ${s.numberSpoken}');
  if (s.publicCallName != null) b.write(', atas nama ${s.publicCallName}');
  b.write(', silakan menuju ${s.counterSpoken}.');
  return b.toString();
}
