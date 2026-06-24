**[English](../en/architecture.md)** | Русский

# Обзор архитектуры

Высокоуровневая архитектура для операторов. Техническая документация: [../../ru/context/architecture.md](../../ru/context/architecture.md).

```mermaid
flowchart TB
  clients["Браузер / AWS CLI / SDK"]
  caddy["Caddy :8080"]
  console["web/console"]
  server["storage-server :9000"]
  meta[(BoltDB или PostgreSQL)]
  fs["objects/ на диске"]
  clients --> caddy
  caddy --> console & server
  server --> meta & fs
```

## Потоки запросов

### Вход в консоль (JWT)

```mermaid
sequenceDiagram
  participant U as Браузер
  participant C as Caddy :8080
  participant S as storage-server
  participant M as БД метаданных
  U->>C: POST /api/v1/admin/login
  C->>S: прокси
  S->>M: проверка учётных данных
  S-->>U: JWT токен
  U->>C: API с Bearer JWT
```

### S3-операции (SigV4)

```mermaid
sequenceDiagram
  participant CLI as S3 клиент
  participant S as storage-server :9000
  participant FS as objects/
  CLI->>S: PutObject (подпись)
  S->>S: проверка SigV4
  S->>FS: запись
  S-->>CLI: 200 OK
```

## Расположение данных

| Путь | Содержимое |
|------|------------|
| `STORAGE_DATA_DIR/objects/` | Байты объектов |
| `metadata.db` или PostgreSQL | Бакеты, пользователи, политики, арендаторы |

## См. также

- [Репликация Gateway](../../ru/user-guide/06-gateway-i-minio.md)
- [Схема БД](../../ru/database.md)
