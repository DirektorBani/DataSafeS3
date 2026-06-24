# Иллюстрации руководства пользователя

В этой папке хранятся **скриншоты реального интерфейса** Датасейф S3 для [руководства пользователя](../README.md).  
Изображения встроены в главы руководства и помогают ориентироваться в веб-консоли, Gateway и Grafana.

> **English:** Screenshots of the live DataSafeS3 UI for the Russian end-user guide. See [user guide](../README.md).

---

## Каталог изображений

| Файл | Что показано | Глава руководства |
|------|--------------|-------------------|
| `login.png` | Экран входа в веб-консоль: поля логина и пароля, кнопка «Sign in» | [01 — Введение и вход](../01-vvedenie-i-vhod.md) |
| `dashboard.png` | Главная страница Dashboard: сводка по хранилищу и навигация | [02 — Главная и бакеты](../02-dashbord-i-bakety.md) |
| `buckets.png` | Список бакетов с демонстрационным бакетом `user-guide-demo` | [02 — Главная и бакеты](../02-dashbord-i-bakety.md) |
| `bucket-detail.png` | Браузер объектов внутри бакета: файл `sample.txt` | [02 — Главная и бакеты](../02-dashbord-i-bakety.md) |
| `gateway.png` | Раздел Gateway, вкладка Connections | [06 — Gateway replication](../../ru/user-guide/06-gateway-i-minio.md) |
| `mfa-profile.png` | Страница Profile: настройка двухфакторной аутентификации (MFA) | [04 — Безопасность и профиль](../04-bezopasnost-i-profil.md) |
| `grafana.png` | Grafana: дашборд «DataSafeS3 Overview» | [07 — Мониторинг и базы данных](../07-monitoring-i-bazy.md) |

---

**Last capture:** 2026-06-19 — `node scripts/capture-screenshots.mjs` (7 PNG, 1280×720)

| Параметр | Значение |
|----------|----------|
| Формат | PNG |
| Размер области съёмки | 1280×720 px (viewport Playwright) |
| Содержимое | Актуальный UI запущенного стека, без разметки-заглушки |

---

## Обновление скриншотов (для сопровождающих)

Снимки генерируются автоматически скриптом [`scripts/capture-screenshots.mjs`](../../../scripts/capture-screenshots.mjs) (Playwright, headless Chromium).

### Предварительные условия

1. Локальный стек запущен (`docker compose up` или эквивалент).
2. Веб-консоль доступна по адресу **http://localhost:8080**.
3. Grafana доступна по адресу **http://localhost:3000** (для `grafana.png`).
4. Учётные данные администратора заданы в `.env`:
   - `STORAGE_ADMIN_USER`
   - `STORAGE_ADMIN_PASSWORD`
5. У учётной записи администратора **отключён MFA** — иначе скрипт не сможет войти.

### Запуск

```cmd
node scripts\capture-screenshots.mjs
```

Скрипт создаёт демо-бакет `user-guide-demo` с файлом `sample.txt`, делает снимки всех экранов и сохраняет их в эту папку. По завершении выводится сводка; при необходимости создаётся `capture-summary.json`.

### Переменные окружения (опционально)

| Переменная | По умолчанию | Назначение |
|------------|--------------|------------|
| `CONSOLE_URL` | `http://localhost:8080` | URL веб-консоли |
| `GRAFANA_URL` | `http://localhost:3000` | URL Grafana |
| `STORAGE_ADMIN_USER` | `admin` | Логин администратора |
| `STORAGE_ADMIN_PASSWORD` | `admin` | Пароль администратора |

Значения по умолчанию совпадают с [.env.example](../../../.env.example) для локальной разработки.

### Вставка в руководство

В markdown-файлах глав используйте относительный путь от файла `.md`:

```markdown
![Описание экрана](login.png)
```
