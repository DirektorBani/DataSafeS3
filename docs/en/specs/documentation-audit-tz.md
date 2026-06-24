English | **[Русский](../../ru/specs/documentation-audit-tz.md)**

# Documentation Audit TZ

**English summary:** Full bilingual documentation audit for `docs/en/`, `docs/ru/`, language pickers, legacy redirects, and root README. Each checklist item is PASS/FAIL with automated grep + manual spot checks.

| Language | Link |
|----------|------|
| **English** | This document (canonical TZ) |
| **Русский** | [documentation-audit-tz.md (RU)](../../ru/specs/documentation-audit-tz.md) |

---

## 1. Scope

| Area | Paths |
|------|-------|
| English tree | `docs/en/**` |
| Russian tree | `docs/ru/**` |
| Language picker | `docs/README.md` |
| Root README links | `README.md` (Documentation section) |
| Legacy redirects | `docs/user-guide/`, `docs/context/`, `docs/specs/`, `docs/integrations/` |
| Shared assets | `docs/images/`, `docs/diagrams/`, `docs/user-guide/images/` |
| Out of scope | `docs/testing/**` (audit reports, not product docs) |

---

## 2. Checklist

Each item: **PASS** or **FAIL** with evidence (command output or file path).

### 2.1 Structure

| ID | Check | PASS criteria |
|----|-------|---------------|
| S1 | Mirror trees | `docs/en/` and `docs/ru/` have the same relative paths (excluding filenames localized by language) |
| S2 | File count parity | Same number of `.md` files in `en/` and `ru/` |
| S3 | Picker works | `docs/README.md` links resolve to `en/README.md` and `ru/README.md` |
| S4 | Root README | `README.md` links to `docs/en/` and `docs/ru/` user guides and context |

### 2.2 Completeness

| ID | Check | PASS criteria |
|----|-------|---------------|
| C1 | EN↔RU pairs | Every RU doc has EN counterpart at mirrored path |
| C2 | Specs TZ translated | All specs in `en/specs/` and `ru/specs/` including this audit TZ |
| C3 | No orphan bilingual content | No full duplicate guides left only in legacy `docs/user-guide/` (stubs OK) |

### 2.3 Translation quality

| ID | Check | PASS criteria |
|----|-------|---------------|
| T1 | No stray RU in `en/` | Body text in `docs/en/` is English (product name «Датасейф S3», bilingual alt text, terminology tables with RU column are allowed) |
| T2 | RU docs in Russian | `docs/ru/` titles and body are Russian |
| T3 | No placeholders | No `[TODO`, `TBD`, `FIXME`, `XXX` in en/ or ru/ |
| T4 | Code blocks unchanged | Commands, ports, env vars identical across language pairs |

### 2.4 Links

| ID | Check | PASS criteria |
|----|-------|---------------|
| L1 | Internal links resolve | All relative `](...)` targets exist from source file directory |
| L2 | Cross-language headers | Line 1 uses correct `../../ru/` or `../../en/` depth from subfolders |
| L3 | Legacy redirects | Stubs in legacy folders point to correct `en/` and `ru/` paths |
| L4 | No wrong legacy paths | No broken `../context/` targets from language trees |

### 2.5 Images

| ID | Check | PASS criteria |
|----|-------|---------------|
| I1 | PNG exists | Every markdown image with a `.png` target path — file exists on disk |
| I2 | Diagram paths | `../../images/` from user-guide README resolves |
| I3 | Screenshot paths | `../../user-guide/images/` from chapter files resolves |

### 2.6 Legacy & duplicates

| ID | Check | PASS criteria |
|----|-------|---------------|
| D1 | English-only context docs | `local-dev`, `project-status`, `performance-review` reachable from en/ru trees |
| D2 | No broken roadmap refs | `performance-review.md` link from roadmap resolves |

---

## 3. Acceptance criteria

Audit **passes** when all checklist items are **PASS**.

Report: `docs/testing/documentation-audit-report.md` with table `ID | Status | Notes | Fixed`.

Minimum score for release: **100% PASS** on L1, L2, I1, S1, S2, C1.

---

## 4. Audit procedure

### 4.1 Automated

```powershell
Get-ChildItem docs/en, docs/ru -Recurse -Filter *.md | Sort-Object FullName
rg -n '\[TODO|TBD|FIXME' docs/en docs/ru
rg -n '[а-яА-ЯёЁ]' docs/en
rg -n '\]\(\.\./ru/' docs/en/context docs/en/user-guide docs/en/specs docs/en/integrations
```

### 4.2 Manual spot checks (3–5 docs)

1. `docs/en/user-guide/README.md`
2. `docs/ru/user-guide/README.md`
3. `docs/en/specs/settings-ui-split-tz.md`
4. `docs/en/context/roadmap.md`
5. `docs/README.md`

### 4.3 Fix policy

Fix all FAIL items in the same commit batch. Do **not** commit `docs/testing/documentation-audit-report.md` unless explicitly requested.

---

*Author: documentation audit · Date: 2026-06-18*
