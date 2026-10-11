**[English](../../en/integrations/kafka.md)** | Русский

# Kafka

Kafka — sink событий только по включению. Установка по умолчанию не задаёт `STORAGE_KAFKA_BROKERS`, не поднимает брокер и не открывает соединение. `VERSION` по-прежнему 1.4.1. Это незарелиженная работа.

Клиент — [franz-go](https://github.com/twmb/franz-go) (`github.com/twmb/franz-go`), чистый Go, без CGO. Выбор записан в [ADR-0002](../../architecture/adr/0002-event-sink-interface.md). Образ контейнера держит клиент во втором статическом бинаре, `/usr/lib/datasafe/datasafe-kafka-bridge`. Процесс сервера модуль не импортирует. Он запускает эту программу только когда `STORAGE_KAFKA_BROKERS` непустой, поэтому процесс по умолчанию не отображает клиент в память и не стартует дочерний процесс. `go build` без `-tags datasafe_sinkexec` линкует клиент в бинарь сервера; `go test` идёт этим путём. В обоих случаях объект клиента, его горутины и любой dial появляются только при непустом списке брокеров.

## Настройка

Пустая переменная выключает sink.

| Переменная | По умолчанию | Смысл |
|------------|--------------|--------|
| `STORAGE_KAFKA_BROKERS` | пусто | `host:port` через запятую. Пусто — sink выключен |
| `STORAGE_KAFKA_TOPIC` | `datasafe.events` | Топик продюсера |
| `STORAGE_KAFKA_CLIENT_ID` | `datasafe` | client id Kafka |
| `STORAGE_KAFKA_TLS` | false | Соединение по TLS. Также включается, если задан файл CA, сертификата или ключа |
| `STORAGE_KAFKA_TLS_CA_FILE` | | PEM-пакет CA |
| `STORAGE_KAFKA_TLS_CERT_FILE` | | Сертификат клиента. Задаётся вместе с ключом |
| `STORAGE_KAFKA_TLS_KEY_FILE` | | Закрытый ключ клиента |
| `STORAGE_KAFKA_SASL_MECHANISM` | | `PLAIN`, `SCRAM-SHA-256` или `SCRAM-SHA-512` |
| `STORAGE_KAFKA_SASL_USERNAME` | | Имя SASL. `STORAGE_KAFKA_SASL_USERNAME_FILE` перекрывает его |
| `STORAGE_KAFKA_SASL_PASSWORD` | | Пароль SASL. `STORAGE_KAFKA_SASL_PASSWORD_FILE` перекрывает его |

`PLAIN` без TLS отказывается стартовать. Ошибка называет `STORAGE_KAFKA_SASL_MECHANISM=PLAIN` и просит `STORAGE_KAFKA_TLS=true` или файлы CA/сертификата/ключа. SCRAM без TLS допустим: пароль не уходит открытым текстом, но канал не шифруется. В промышленной среде для SCRAM включайте TLS.

Значение `*_FILE` читается с диска, хвостовой перевод строки срезается. Отсутствующий файл роняет старт. Содержимое файла в Activity не пишется.

Настройки продюсера, которые не являются переменными окружения:

- `acks=all`
- идемпотентный produce (дефолт franz-go; процесс его не отключает)
- ключ записи `tenant/bucket/key` (слэши внутри ключа объекта сохраняются; ключ объекта читайте из JSON)
- разделитель разделов: sticky key, один ключ объекта остаётся в одном разделе

Лабораторный брокер не входит в профили установщика по умолчанию:

```bash
./install.sh --yes --profiles core,kafka
```

Подключается `deploy/compose/docker-compose.kafka.yml`: один брокер KRaft, образ `apache/kafka:3.9.2`, bootstrap внутри стека `kafka:19092`, порт хоста `localhost:9092`. Оверлей задаёт `storage-server` переменную `STORAGE_KAFKA_BROKERS=kafka:19092`. Контейнер `kafka-init` после готовности брокера выполняет `kafka-topics.sh --create --if-not-exists --topic datasafe.events`. У брокера `auto.create.topics.enable=false`. Windows: `.\install.ps1 -Yes -Profiles core,kafka`.

Sink топики не создаёт. Переменной `STORAGE_KAFKA_AUTO_CREATE_TOPIC` нет. Создайте `STORAGE_KAFKA_TOPIC` до первого события. Пока топика нет, produce падает, а outbox повторяет попытку. Ошибка на странице **Настройки → Доставка событий** и в `GET /api/v1/events/delivery` (`sinks[].last_error`) — `topic does not exist`, затем текст брокера `UNKNOWN_TOPIC_OR_PARTITION`.

Смена `STORAGE_KAFKA_BROKERS` между стартами процесса — действие Activity `kafka_brokers_changed`. Имя ресурса — новый список брокеров. Секреты SASL не пишутся.

## Формат сообщения

Kafka не использует тело webhook или NATS. Те тела остаются прежними.

```json
{
  "Records": [
    {
      "eventVersion": "2.2",
      "eventSource": "datasafe:s3",
      "eventName": "s3:ObjectCreated:Put",
      "eventTime": "2026-10-11T12:00:00.000Z",
      "s3": {
        "bucket": { "name": "photos" },
        "object": {
          "key": "a/b.jpg",
          "size": 12,
          "eTag": "",
          "versionId": ""
        }
      }
    }
  ],
  "datasafe": {
    "tenant_id": "",
    "event_id": "01H...",
    "schema_version": 1
  }
}
```

| Сохранённый `event_type` | `eventName` |
|--------------------------|-------------|
| `ObjectCreated` | `s3:ObjectCreated:Put` |
| `ObjectDeleted` | `s3:ObjectRemoved:Delete` |
| `MultipartCompleted` | `s3:ObjectCreated:CompleteMultipartUpload` |
| `BucketCreated` | `s3:BucketCreated` |
| `BucketDeleted` | `s3:BucketRemoved` |
| `UserCreated` | `datasafe:UserCreated` |
| `ShareCreated` | `datasafe:ShareCreated` |
| `TestEvent` | `datasafe:TestEvent` |

`ObjectCreated` покрывает и copy, и put. Сохранённый тип их не различает. `key` — сырой ключ объекта, не URL-кодирование уведомлений AWS. `size` и `eTag` берутся из сохранённого payload, если поля есть; иначе size равен `0`, а `eTag` пустой. `versionId` — version id строки outbox. `schema_version` равен `1`.

## Доставка

Доставка at-least-once. Смена лидера во время publish или рестарт после того, как брокер принял запись, но строка outbox ещё не сохранена, может доставить тот же `event_id` ещё раз. Идемпотентный produce убирает дубликаты повторов внутри одной сессии клиента. Он не убирает дубликаты через рестарт. Потребителю стоит считать `datasafe.event_id` ключом идемпотентности.

Порядок — на каждый sink и каждый ключ объекта. Диспетчер не публикует в Kafka более позднюю строку с тем же tenant, бакетом и ключом, пока более ранняя ещё `pending` для sink Kafka. Застрявший ключ не блокирует другие ключи и не блокирует webhook или NATS для этого ключа. Пустой ключ объекта не упорядочивается (`UserCreated`, создание и удаление бакета).

Если попытка Kafka более ранней строки стала `dead` (исчерпаны попытки или терминальная ошибка), более поздние строки этого ключа публикуются. Мёртвая — нет. Это пропуск. Нельзя считать, что в топике есть каждое изменение объекта.

`GET /api/v1/events/delivery` (администратор) возвращает `connected`, `degraded` или `disabled`, последнюю ошибку и глубину очереди. Для Kafka `connected` значит: sink зарегистрирован, последняя публикация не упала, и ping этого запроса успешен. Это не лаг потребителя. `POST /api/v1/events/test` ставит один `TestEvent` каждому зарегистрированному sink. Страница консоли — **Настройки → Доставка событий**.
