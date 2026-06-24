**[English](../../en/user-guide/08-federation-and-cluster.md)** | Русский

# 8. Federation и Cluster

[← Мониторинг](07-monitoring-i-bazy.md) | [К содержанию](README.md)

> Разделы **Federation** и **Cluster** доступны только **администратору**.

> **Однонодовый режим по умолчанию:** Federation и Cluster **не** обеспечивают multi-AZ высокую доступность. Federation добавляет **S3 read proxy** (GetObject + ListObjectsV2) между зарегистрированными пирами — см. ниже. Паттерны HA: [масштабирование](../../operations-guide/ru/scaling.md), [эталон 2-node](../../operations-guide/ru/reference-deployment-2node.md).

---

## Federation (федерация)

**Federation** — **реестр других серверов DataSafeS3** и **S3 proxy** для чтения через зарегистрированные пиры.

### Что работает сегодня

| Возможность | Статус |
|-------------|--------|
| Регистрация удалённых endpoint DataSafeS3 | **Реализовано** — Administration → Federation |
| S3 **GetObject** через пир | **Реализовано** — если объект не найден локально |
| S3 **ListObjectsV2** prefix proxy | **Реализовано** — список объектов на удалённом пире по префиксу |
| Фоновый sync worker | **Реализовано** — задания federation sync в консоли |
| Автоматическое размещение данных / глобальный поиск | **В планах** |

### Зачем регистрировать удалённый кластер

- учёт филиалов или резервных площадок;
- единый реестр endpoint'ов;
- основа для будущего сквозного поиска (пока в MVP).

### Как добавить

1. **Administration → Federation**.
2. **Register cluster**.
3. Укажите:
   - **имя** (например «Офис Москва»);
   - **endpoint** (URL S3/API другого Датасейф S3);
   - **region** (регион, например `us-east-1`).
4. Сохраните.

> Сейчас это **реестр конфигурации** — автоматическая репликация между двумя Датасейф S3 через Federation отличается от Gateway (Gateway копирует в **любое** S3, в том числе external S3).

---

## Cluster (кластер)

**Cluster** показывает, работает ли Датасейф S3 как **один сервер** или в **распределённом режиме** (несколько нод).

### Что видно на странице Cluster

| Поле | Описание |
|------|----------|
| Distributed mode | Включён ли распределённый режим |
| Erasure coding | Планируется ли erasure coding (пока флаг, алгоритм в roadmap) |
| Disk paths | Пути к дискам (для будущего multi-disk) |
| Nodes | Список нод и их статус |

### Однонодовый режим (по умолчанию)

Для большинства установок достаточно **одного сервера** — это основной production-путь:

- одна нода `local` @ `localhost:9000`, status `healthy`;
- все данные на одном хосте.

### Распределённый режим (MVP)

В **Settings → Cluster** администратор может:

- включить **Distributed mode**;
- указать **disk paths** (по одному пути на строку);
- отметить **Erasure coding planned**.

> Полноценный HA-кластер с erasure coding **ещё в разработке**. Сейчас это конфигурация и мониторинг «на будущее», а не автоматическое распределение данных.

### Настройка через API

Для продвинутых сценариев параметры кластера можно задать через REST API `PUT /api/v1/settings/system` — см. [architecture.md](../context/architecture.md).

---

## Сравнение: Gateway vs Federation vs Cluster

| Функция | Назначение |
|---------|------------|
| **Gateway** | Копирование во **внешнее S3** — **работает сегодня** |
| **Federation** | Реестр + **GetObject / ListObjectsV2 proxy** к другим площадкам DataSafeS3 — MVP |
| **Cluster** | Несколько **нод одного** DataSafeS3 — основа + UI |

---

## Полезные ссылки

- [Руководство: Gateway](06-gateway-i-minio.md)
- [Техническая архитектура](../context/architecture.md)
- [Roadmap](../context/roadmap.md)
- [Статус проекта](../context/project-status.md)

---

[← К содержанию](README.md)
