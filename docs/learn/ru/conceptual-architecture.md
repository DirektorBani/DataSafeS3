**[English](../en/conceptual-architecture.md)** | Русский

# Концептуальная архитектура

Высокоуровневая архитектура для понимания продукта. Детали реализации: [логическая архитектура](../../ru/context/architecture.md) и [схема БД](../../ru/database.md).

---

## Концептуальная архитектура (30 секунд)

```mermaid
flowchart TB
  people[Пользователи и приложения]
  ds[Платформа DataSafeS3]
  disks[Ваше хранилище]
  people --> ds --> disks
```

DataSafeS3 находится между людьми (браузер, S3-клиенты, автоматизация) и дисковым хранилищем. Идентичность, политики, аудит и мониторинг оборачивают каждый запрос.

---

## Логическая архитектура

```mermaid
flowchart TB
  clients[Браузер S3 CLI SDK]
  caddy[Caddy reverse proxy]
  console[Веб-консоль]
  server[storage-server]
  subgraph services [Основные сервисы]
    s3[S3 API SigV4]
    admin[Admin REST API]
    gw[Gateway worker]
  end
  meta[(Хранилище метаданных)]
  objects[Файлы объектов на диске]
  clients --> caddy & server
  caddy --> console
  caddy --> server
  server --> s3 & admin & gw
  server --> meta & objects
```

| Подсистема | Роль |
|------------|------|
| **Веб-консоль** | Администрирование и self-service пользователей |
| **storage-server** | S3 API, Admin API, worker репликации, метрики |
| **Метаданные** | Пользователи, бакеты, политики, тенанты, аудит |
| **Объектное хранилище** | Файлы под `STORAGE_DATA_DIR/objects/` |

---

## Развёртывание — один узел

```mermaid
flowchart LR
  subgraph host [Один хост Docker Compose]
    caddy[Caddy :8080]
    srv[storage-server :9000]
    bolt[(BoltDB по умолчанию)]
    prom[Prometheus]
    graf[Grafana]
  end
  disk[(Локальный том)]
  srv --> bolt & disk
  prom --> srv
  graf --> prom
```

Типично для оценки и небольших команд: одна VM, Compose, опционально PostgreSQL для метаданных.

Руководство: [Первый запуск](../../getting-started/ru/first-run.md)

---

## Production-архитектура

```mermaid
flowchart TB
  subgraph k8s [Кластер Kubernetes]
    ingConsole[Ingress консоль]
    ingS3[Ingress S3 API]
    pods[Поды DataSafeS3]
    pg[(PostgreSQL StatefulSet)]
    mon[Prometheus Grafana]
  end
  pvc[(PersistentVolume objects)]
  pods --> pg & pvc
  ingConsole & ingS3 --> pods
  mon --> pods
```

Production: TLS, PostgreSQL, резервное копирование, алерты, смена bootstrap-учётных данных. Опционально HA: [эталон 2-node](../../operations-guide/ru/reference-deployment-2node.md), Helm `values-ha.yaml`.

Руководство: [Helm chart](../../../deploy/helm/datasafe/README.md) · [Эксплуатация](../../operations-guide/ru/README.md)

---

## Multi-site — репликация Gateway

```mermaid
flowchart LR
  primary[Основная площадка DataSafeS3]
  gateway[Асинхронный Gateway]
  remote[Внешнее S3-совместимое хранилище]
  primary --> gateway --> remote
```

Основная площадка принимает записи локально. Gateway реплицирует объекты во внешний бакет для off-site копий и DR.

Руководство: [Репликация](../../administrator-guide/ru/replication.md) · [Gateway](../../ru/context/gateway.md)

---

## Архитектура аутентификации

```mermaid
flowchart TB
  subgraph consoleLogin [Вход в консоль]
    local[Локальный пароль]
    ldap[LDAP каталог]
    oidc[OIDC провайдер]
  end
  jwt[JWT сессия]
  mfa[MFA TOTP опционально]
  local & ldap & oidc --> jwt
  jwt --> mfa
  s3[S3 ключи SigV4]
  apps[Приложения] --> s3
```

| Путь | Сценарий |
|------|----------|
| **LDAP** | Корпоративный каталог и группы |
| **OIDC / SSO** | Единый вход через IdP |
| **MFA** | Второй фактор для консоли |
| **S3 keys** | Доступ приложений к object API |

Руководства: [LDAP](../../administrator-guide/ru/ldap.md) · [OIDC](../../administrator-guide/ru/oidc.md) · [MFA](../../administrator-guide/ru/mfa.md)

---

## См. также

- [Что такое DataSafeS3?](../../getting-started/ru/what-is-datasafe.md)
- [Почему DataSafeS3?](../../ru/why-datasafe.md)
- [Сценарии](../../use-cases/README.md)
