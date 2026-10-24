# Совместимость с S3

English: [compatibility.md](../compatibility.md).

DataSafeS3 говорит на HTTP API S3, которым пользуются программы резервного копирования и хранилища артефактов на одном узле: path-style URL, SigV4, метаданные Bolt без отдельной базы. Это не полная реализация каждого метода Amazon S3.

Автоматическая проверка: `go test ./internal/api/s3/ -run TestS3ClientCompatibility`. Она запускает те из `aws`, `mc`, `rclone` и `restic`, что есть в `PATH`. CI задаёт `S3_COMPAT_REQUIRE=aws,mc,rclone,restic`, и отсутствующий клиент роняет задание. `TestAWSSDKDeleteObjects` вызывает DeleteObjects через aws-sdk-go-v2.

## Клиенты

| Клиент | Что делает тест | Результат |
|--------|-----------------|-----------|
| aws-cli v2 | Бакет, put/get/head, list-objects-v2, copy, presign, составная загрузка, версионирование, `s3 rm --recursive` | ожидается успех |
| MinIO Client `mc` | `mb`, `cp`, `ls`, `rm --recursive` | ожидается успех |
| rclone | `copyto`, `ls`, `delete` у провайдера `Other` | ожидается успех |
| restic | `init`, `backup`, `snapshots`, `forget --prune` | ожидается успех |
| aws-sdk-go-v2 | `CreateBucket`, `PutObject`, `DeleteObjects`, `GetObject` | модульный тест, идёт всегда |

`s3 rm --recursive`, `mc rm --recursive`, удаления rclone и prune restic зависят от `POST /{bucket}?delete` (DeleteObjects). До 1.7.0 этот вызов отвечал HTTP 405.

## Операции

| Операция | Поддержка |
|----------|-----------|
| ListBuckets, CreateBucket, DeleteBucket, HeadBucket, GetBucketLocation | да |
| PutObject, GetObject, HeadObject, DeleteObject, CopyObject | да |
| ListObjects и ListObjectsV2 (не больше 1000, продолжение) | да |
| Составная загрузка: начало, часть, завершение, отмена, список | да |
| DeleteObjects (до 1000 ключей, Quiet, ошибка по ключу, Content-MD5 или `x-amz-checksum-*`) | да, с 1.7.0 |
| Версионирование бакета и маркеры удаления | да |
| Список версий объекта | да |
| Метки объекта | да |
| Политика бакета (подмножество IAM, включая Deny и `aws:SourceIp`) | да |
| Предподписанный GET | да, ключом пользователя. Корневой ключ установки предподпись не делает |
| Предподписанный PUT | частично. `Content-Type` может быть не подписан. Предподписанный DELETE отклоняется |
| Заголовки шифрования бакета и объекта (`AES256`, `aws:kms`) | да. `aws:kms` использует Vault Transit только когда провайдер настроен, иначе ключ сервера |
| Object Lock, срок хранения, юридическое удержание | да. `x-amz-bypass-governance-retention: true` и право `s3:BypassGovernanceRetention` обходят GOVERNANCE. COMPLIANCE и удержание не обходятся |
| Истечение жизненного цикла по префиксу | да. Переход, истечение нетекущих версий и фильтр по меткам отклоняются |
| Конфигурация уведомлений бакета | сохраняется и доставляется через очередь, когда приёмник настроен |
| Хвостовые контрольные суммы aws-chunked (CRC32, CRC32C, SHA-1, SHA-256, CRC64NVME) | да |
| ACL объекта и бакета, сайт S3, API CORS, select, restore, API конфигурации репликации и инвентаризации, accelerate, requester pays, object lambda | нет |
| Виртуальный хост бакета (`bucket.host`) | нет. Нужен path-style (`addressing_style = path`) |
| Потоковая загрузка AWS chunked | да, разбирается до записи |

Удаление отсутствующего ключа в DeleteObjects считается успехом. Отсутствующий `versionId` для ключа — `NoSuchVersion`. У правильно сформированного DeleteObjects код HTTP 200, даже если часть ключей не удалилась.

## Чем это отличается от Amazon S3 и MinIO

- Байты на диске принадлежат одному процессу. Дополнительные pod делят метаданные через Postgres и копируют байты соседям. Это не erasure MinIO между серверами.
- ETag одиночного PUT — MD5 сохранённого тела для открытого текста. ETag шифрования и составной загрузки не равен MD5 открытого текста.
- GET локального объекта не сверяет байты с сохранённым ETag. Копия, принятая от соседа, сверяется.
- Политика бакета — то подмножество, что описано в заметках переноса IAM, а не весь язык IAM.
