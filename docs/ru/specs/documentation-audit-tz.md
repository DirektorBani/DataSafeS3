**[English](../../en/specs/documentation-audit-tz.md)** | Русский

# ТЗ на аудит документации

**English summary:** Full bilingual documentation audit for `docs/en/`, `docs/ru/`, language pickers, legacy redirects, and root README.

| Язык | Ссылка |
|------|--------|
| **English** | [documentation-audit-tz.md (EN)](../../en/specs/documentation-audit-tz.md) |
| **Русский** | Этот документ |

---

## 1. Область

| Область | Пути |
|---------|------|
| Английское дерево | `docs/en/**` |
| Русское дерево | `docs/ru/**` |
| Выбор языка | `docs/README.md` |
| Ссылки в корневом README | `README.md` (раздел Documentation) |
| Legacy-редиректы | `docs/user-guide/`, `docs/context/`, `docs/specs/`, `docs/integrations/` |
| Общие ресурсы | `docs/images/`, `docs/diagrams/`, `docs/user-guide/images/` |
| Вне scope | `docs/testing/**` |

---

## 2. Чеклист

Каждый пункт: **PASS** или **FAIL** с доказательством.

### 2.1 Структура (S1–S4)

Зеркальные деревья `en/` и `ru/`, одинаковое число `.md`, рабочий picker, ссылки в корневом README.

### 2.2 Полнота (C1–C3)

Пары EN↔RU, переведённые specs TZ, legacy `user-guide/` только stubs.

### 2.3 Качество перевода (T1–T4)

Нет лишней кириллицы в `en/` (кроме имени продукта и таблиц терминов), русский текст в `ru/`, нет placeholder, код-блоки идентичны.

### 2.4 Ссылки (L1–L4)

Все относительные ссылки резолвятся; заголовки EN|RU с правильной глубиной `../../`; legacy stubs корректны.

### 2.5 Изображения (I1–I3)

PNG существуют; пути `../../images/` и `../../user-guide/images/` корректны.

### 2.6 Наследие (D1–D2)

`local-dev`, `project-status`, `performance-review` доступны из en/ru; ссылка из roadmap работает.

---

## 3. Критерии приёмки

Аудит **пройден**, когда все пункты **PASS**. Отчёт: `docs/testing/documentation-audit-report.md` (не коммитить без запроса).

---

## 4. Процедура

Автоматика: сравнение списков файлов, grep placeholder/кириллицы, проверка resolver ссылок. Ручная выборка 3–5 документов.

---

*Дата: 2026-06-18*
