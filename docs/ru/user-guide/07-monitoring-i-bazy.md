**[English](../../en/user-guide/07-monitoring-and-databases.md)** | Русский

# 7. Мониторинг, PostgreSQL и DBeaver

[← Gateway](06-gateway-i-minio.md) | [К содержанию](README.md) | Далее: [Federation →](08-federation-i-cluster.md)

---

## Grafana — простой мониторинг

**Grafana** — сайт с **графиками** работы Датасейф S3: сколько запросов, сколько места занято, нет ли ошибок.

| Параметр | Значение (локально) |
|----------|---------------------|
| URL | http://localhost:3000 |
| Логин | `admin` |
| Пароль | `admin` |

### Готовые дашборды

После входа откройте:

| Дашборд | URL |
|---------|-----|
| **Обзор системы** | http://localhost:3000/d/datasafe-overview/datasafe-overview |
| **Бакеты** (выбор одного или нескольких) | http://localhost:3000/d/datasafe-buckets/datasafe-buckets |

![Grafana — дашборд Датасейф S3 Overview](../../user-guide/images/grafana.png)

На дашборде обзора вы увидите:

- количество HTTP-запросов и ошибок;
- время ответа сервера;
- объём хранилища, число бакетов и объектов;
- операции S3 чтения/записи;
- метрики хоста: CPU, диск, память, сеть.

Дашборд **Бакеты** добавляет переменную **Bucket** (мультивыбор) для детализации по объектам, размеру и S3-операциям в выбранных бакетах.

> Grafana нужна **администратору** для наблюдения за здоровьем системы. Обычному пользователю достаточно раздела **Usage** в консоли.

### Prometheus

Сбор метрик идёт через **Prometheus** (http://localhost:9090). Grafana берёт данные оттуда. Вручную заходить в Prometheus обычно не нужно.

---

## Bolt vs PostgreSQL — что выбрать?

Датасейф S3 хранит **справочную информацию** (список бакетов, пользователей, настройки) отдельно от самих файлов. Есть два варианта:

| | **Bolt** (по умолчанию) | **PostgreSQL** |
|---|-------------------------|----------------|
| **Простыми словами** | Один файл базы на диске | Полноценная SQL-база |
| **Когда подходит** | Один сервер, простая установка | Production, много данных, поиск, аналитика |
| **Настройка** | Ничего дополнительно | Профиль `postgres` в Docker |
| **Файл / сервер** | `metadata.db` в папке data | Контейнер PostgreSQL |

### Bolt — быстрый старт

В `.env` (или по умолчанию):

```env
STORAGE_METADATA_BACKEND=bolt
```

Запуск:

```cmd
docker compose up -d --build
```

### PostgreSQL — production

1. В `.env`:

```env
STORAGE_METADATA_BACKEND=postgres
STORAGE_POSTGRES_HOST=postgres
STORAGE_POSTGRES_USER=datasafe
STORAGE_POSTGRES_PASSWORD=datasafe
STORAGE_POSTGRES_DB=datasafe
STORAGE_POSTGRES_PUBLISH_PORT=5433
```

2. Запуск:

```cmd
docker compose --profile postgres up -d --build
```

> Перенос данных с Bolt на PostgreSQL: команда `storage-server migrate-boltdb` (см. [README](../../../README.md) или [local-dev.md](../context/local-dev.md)).

---

## DBeaver — подключение к PostgreSQL

**DBeaver** — программа для просмотра таблиц базы данных (для администраторов и разработчиков).

### Важно про порт 5433 на Windows

Если на компьютере уже установлен PostgreSQL, он часто занимает порт **5432**.  
Docker-контейнер Датасейф S3 тогда публикуется на **5433**.

В `.env` задайте:

```env
STORAGE_POSTGRES_PUBLISH_PORT=5433
```

Пересоздайте контейнер:

```cmd
docker compose --profile postgres up -d postgres
```

### Параметры подключения в DBeaver

| Поле | Значение |
|------|----------|
| Тип | PostgreSQL |
| Host | `localhost` |
| Port | **5433** (или 5432, если 5433 не задан) |
| Database | `datasafe` |
| Username | `datasafe` |
| Password | `datasafe` |
| SSL | отключить (`sslmode=disable`) |

**JDBC URL:**

```
jdbc:postgresql://localhost:5433/datasafe?sslmode=disable
```

### Проверка с командной строки

```cmd
docker run --rm -e PGPASSWORD=datasafe postgres:16-alpine psql -h host.docker.internal -p 5433 -U datasafe -d datasafe -c "SELECT 1;"
```

Должно вернуть `1`.

---

## Что дальше?

- [Federation и Cluster →](08-federation-i-cluster.md)
- [Техническая документация →](../context/)
