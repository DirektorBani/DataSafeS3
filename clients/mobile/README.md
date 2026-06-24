# DataSafeS3 Mobile (Phase 4)

Flutter MVP: sign in, browse buckets/objects, upload files.

## Prerequisites

- [Flutter SDK](https://flutter.dev/docs/get-started/install) 3.22+

## Run

```bash
cd clients/mobile
flutter pub get
flutter run
```

**Android emulator:** use server URL `http://10.0.2.2:8080` (host machine).

**iOS simulator:** use `http://localhost:8080` if the API runs on the Mac host.

## Scope (Phase 4 MVP)

- JWT login (`POST /api/v1/admin/login`)
- List buckets and objects
- Upload via `PUT /api/v1/buckets/{bucket}/objects/{key}`
- Download preview — planned; use mobile-web for quick tests

For a browser-based mobile UI without Flutter, see [../mobile-web/README.md](../mobile-web/README.md).
