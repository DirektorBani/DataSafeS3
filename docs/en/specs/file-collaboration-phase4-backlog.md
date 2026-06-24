# File collaboration — Phase 4 backlog

**Status:** Deferred (Phase 3 desktop sync shipped first)  
**Spec:** [file-collaboration-tz.md](./file-collaboration-tz.md) Appendix B  
**Last updated:** 2026-06-23

---

## Goal

Mobile access to DataSafeS3 file workspace: browse shared/owned buckets, upload files, optional lightweight PWA — without claiming production background sync or OS file-provider integration.

---

## Backlog (ordered)

| ID | Item | Priority | Notes |
|----|------|----------|-------|
| M4-1 | Flutter: download / open file preview | P0 | MVP gap in `clients/mobile` |
| M4-2 | Flutter: shared buckets + prefix grants UX | P0 | Mirror web «Shared with me» + `shared_prefixes` |
| M4-3 | mobile-web PWA: production build + install manifest | P1 | Today dev-only Vite server |
| M4-4 | Mobile JWT refresh / secure token storage | P1 | Keychain / EncryptedSharedPreferences |
| M4-5 | Upload queue + retry (foreground) | P1 | Large files, flaky networks |
| M4-6 | In-app notifications deep-link to bucket/prefix | P2 | Reuse `/api/v1/notifications` |
| M4-7 | Recent items on mobile home | P2 | `GET /api/v1/recent` |
| M4-8 | Background sync (iOS/Android) | P3 | Out of initial Phase 4 scope |
| M4-9 | iOS Files / Android SAF integration | P3 | Requires native bridges |
| M4-10 | Push notifications (APNs/FCM) | P3 | Server work + secrets |
| M4-11 | Store-ready builds (signed APK/IPA) | P2 | CI + assets |
| M4-12 | EN/RU strings in mobile clients | P1 | Match console i18n |

---

## Out of scope (unchanged)

- Real-time co-editing
- Replacing web console for admins
- Desktop sync (Phase 3 — **implemented**)

---

## Dependencies

- Phase 1–2 web API (buckets, access, notifications, recent) — **done**
- Phase 3 sync patterns (conflict policy, delete sync) — reference only for future mobile offline

---

## Acceptance (when Phase 4 starts)

1. User can log in on phone, see owned + shared buckets.
2. User can list objects under granted prefix and upload a file.
3. User can download/open a file on device.
4. mobile-web PWA installable on LAN demo.
5. Docs EN/RU updated; no competitor positioning.

---

**RU mirror:** [file-collaboration-phase4-backlog.md](../../ru/specs/file-collaboration-phase4-backlog.md)
