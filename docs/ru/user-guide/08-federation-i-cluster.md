**[English](../../en/user-guide/08-federation-and-cluster.md)** | Русский

# 8. Federation и Cluster

[← Мониторинг](07-monitoring-i-bazy.md) | [К содержанию](README.md)

> Разделы **Федерация** и **Кластеры** — только для **администратора**.

> **Single-node по умолчанию:** эти функции **не заменяют** production multi-AZ HA. Паттерны HA: [масштабирование](../../operations-guide/ru/scaling.md), [эталон 2-node](../../operations-guide/ru/reference-deployment-2node.md).

---

## Кластеры (доверенные площадки)

Страница **Кластеры** — центр управления **multi-site trust** и **репликацией между инсталляциями** DataSafeS3.

### Что доступно сегодня

| Возможность | Статус |
|-------------|--------|
| Просмотр **локального кластера** (id, endpoint) | **Реализовано** |
| **Join-токен** (`dsjoin_*`) | **Реализовано** — одно использование, 15 мин |
| **Pairing** с другой площадкой (авто mTLS) | **Реализовано** — без ручного отпечатка |
| Список **trusted remote** (статус, срок cert) | **Реализовано** |
| **Revoke** remote | **Реализовано** |
| **Правила репликации** на remote buckets | **Реализовано** (v1.1.0) |
| **HA-ноды** (leader / standby) | При `STORAGE_HA_ENABLED=true` |

### Типичный сценарий — два офиса

1. Admin **офиса A** → **Кластеры** → join-токен.
2. Admin **офиса B** → URL A + токен → подключить.
3. На A → remote → правило (`documents` → `documents-dr`).
4. Загрузка файла на A → проверка на B.

Подробно: [Администратор — trusted clusters](../../administrator-guide/ru/trusted-clusters.md) · [Operations — trusted clusters](../../operations-guide/ru/trusted-clusters.md)

### Локальный HA vs trusted remote

| На странице «Кластеры» | Значение |
|------------------------|----------|
| Локальный кластер + список HA-нод | **Ноды этой площадки** |
| Trusted remote clusters | **Отдельная инсталляция** DataSafeS3 после pairing |

---

## Federation (федерация)

**Федерация** — read proxy (GetObject + ListObjectsV2). **Не копирует** записи — для этого **Кластеры**.

### Что работает

| Возможность | Статус |
|-------------|--------|
| Регистрация remote endpoint | **Реализовано** |
| Привязка peer к **кластеру** (local или trusted remote) | **Реализовано** |
| GetObject / ListObjectsV2 через peer | **Реализовано** |
| Federation sync jobs | **Реализовано** |

### Как добавить peer

1. **Федерация** → **Зарегистрировать кластер**.
2. Имя, endpoint, region.
3. Выбрать **Кластер** в dropdown.
4. Сохранить.

---

## Сравнение

| Функция | Назначение |
|---------|------------|
| **Gateway** | Копия во **внешнее S3** |
| **Кластеры** | **Trust** + **репликация объектов** (mTLS) |
| **Federation** | **Чтение** с другой площадки |
| **Site replication** | Legacy AK/SK (lab) |

---

## Ссылки

- [Модели репликации](../../administrator-guide/ru/replication.md)
- [Trusted clusters (operations)](../../operations-guide/ru/trusted-clusters.md)
- [HA reference](../../operations-guide/ru/reference-deployment-2node.md)

---

[← К содержанию](README.md)
