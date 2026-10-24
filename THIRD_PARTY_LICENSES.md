# Лицензии третьих сторон

Версия DataSafeS3: **1.7.1**. DataSafeS3 распространяется по Apache-2.0 (см. [LICENSE](LICENSE)). Ниже — компоненты, которые входят в поставку или ставятся рядом по вашему выбору. Списки Go и npm сформированы автоматически из SBOM релиза (`scripts/sbom.sh`, `scripts/gen-third-party-licenses.py`); полные тексты лицензий лежат в исходных репозиториях модулей и в SBOM.

## 1. Go-модули в бинарнике storage-server

| Модуль | Версия | Лицензия |
|---|---|---|
| `github.com/aws/aws-sdk-go-v2` | v1.42.0 | Apache-2.0 |
| `github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream` | v1.7.13 | Apache-2.0 |
| `github.com/aws/aws-sdk-go-v2/config` | v1.32.25 | Apache-2.0 |
| `github.com/aws/aws-sdk-go-v2/credentials` | v1.19.24 | Apache-2.0 |
| `github.com/aws/aws-sdk-go-v2/feature/ec2/imds` | v1.18.29 | Apache-2.0 |
| `github.com/aws/aws-sdk-go-v2/internal/configsources` | v1.4.29 | Apache-2.0 |
| `github.com/aws/aws-sdk-go-v2/internal/endpoints/v2` | v2.7.29 | Apache-2.0 |
| `github.com/aws/aws-sdk-go-v2/internal/v4a` | v1.4.30 | Apache-2.0 |
| `github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding` | v1.13.12 | Apache-2.0 |
| `github.com/aws/aws-sdk-go-v2/service/internal/checksum` | v1.9.22 | Apache-2.0 |
| `github.com/aws/aws-sdk-go-v2/service/internal/presigned-url` | v1.13.29 | Apache-2.0 |
| `github.com/aws/aws-sdk-go-v2/service/internal/s3shared` | v1.19.29 | Apache-2.0 |
| `github.com/aws/aws-sdk-go-v2/service/s3` | v1.103.3 | Apache-2.0 |
| `github.com/aws/aws-sdk-go-v2/service/signin` | v1.2.0 | Apache-2.0 |
| `github.com/aws/aws-sdk-go-v2/service/sso` | v1.31.3 | Apache-2.0 |
| `github.com/aws/aws-sdk-go-v2/service/ssooidc` | v1.36.6 | Apache-2.0 |
| `github.com/aws/aws-sdk-go-v2/service/sts` | v1.43.3 | Apache-2.0 |
| `github.com/aws/smithy-go` | v1.27.2 | Apache-2.0 |
| `github.com/Azure/go-ntlmssp` | v0.1.1 | MIT |
| `github.com/beorn7/perks` | v1.0.1 | MIT |
| `github.com/boombuler/barcode` | v1.0.1-0.20190219062509-6c824513bacc | MIT |
| `github.com/cespare/xxhash/v2` | v2.3.0 | MIT |
| `github.com/fxamacker/cbor/v2` | v2.8.0 | MIT |
| `github.com/go-asn1-ber/asn1-ber` | v1.5.8-0.20250403174932-29230038a667 | MIT |
| `github.com/go-ldap/ldap/v3` | v3.4.13 | MIT |
| `github.com/go-webauthn/webauthn` | v0.12.3 | BSD-3-Clause |
| `github.com/go-webauthn/x` | v0.1.20 | BSD-3-Clause |
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | MIT |
| `github.com/google/go-tpm` | v0.9.8 | Apache-2.0 |
| `github.com/google/uuid` | v1.6.0 | BSD-3-Clause |
| `github.com/jackc/pgpassfile` | v1.0.0 | MIT |
| `github.com/jackc/pgservicefile` | v0.0.0-20240606120523-5a60cdf6a761 | MIT |
| `github.com/jackc/pgx/v5` | v5.10.0 | MIT |
| `github.com/jackc/puddle/v2` | v2.2.2 | MIT |
| `github.com/klauspost/compress` | v1.20.0 | Apache-2.0 |
| `github.com/klauspost/cpuid/v2` | v2.2.8 | MIT |
| `github.com/klauspost/reedsolomon` | v1.12.4 | MIT |
| `github.com/mitchellh/mapstructure` | v1.5.0 | MIT |
| `github.com/munnerz/goautoneg` | v0.0.0-20191010083416-a7dc8b61c822 | BSD-3-Clause |
| `github.com/nats-io/nats.go` | v1.51.0 | Apache-2.0 |
| `github.com/nats-io/nkeys` | v0.4.16 | Apache-2.0 |
| `github.com/nats-io/nuid` | v1.0.1 | Apache-2.0 |
| `github.com/pierrec/lz4/v4` | v4.1.30 | BSD-3-Clause |
| `github.com/pquerna/otp` | v1.5.0 | Apache-2.0 |
| `github.com/prometheus/client_golang` | v1.23.2 | Apache-2.0 |
| `github.com/prometheus/client_model` | v0.6.2 | Apache-2.0 |
| `github.com/prometheus/common` | v0.66.1 | Apache-2.0 |
| `github.com/prometheus/procfs` | v0.16.1 | Apache-2.0 |
| `github.com/twmb/franz-go` | v1.22.1 | BSD-3-Clause |
| `github.com/twmb/franz-go/pkg/kmsg` | v1.14.0 | BSD-3-Clause |
| `github.com/x448/float16` | v0.8.4 | MIT |
| `go.etcd.io/bbolt` | v1.3.10 | MIT |
| `go.yaml.in/yaml/v2` | v2.4.2 | Apache-2.0 |
| `golang.org/x/crypto` | v0.58.0 | BSD-3-Clause |
| `golang.org/x/oauth2` | v0.36.0 | BSD-3-Clause |
| `golang.org/x/sync` | v0.24.0 | BSD-3-Clause |
| `golang.org/x/sys` | v0.49.0 | BSD-3-Clause |
| `golang.org/x/text` | v0.43.0 | BSD-3-Clause |
| `google.golang.org/protobuf` | v1.36.8 | BSD-3-Clause |

## 2. npm-зависимости консоли (production)

| Пакет | Версия | Лицензия |
|---|---|---|
| `@babel/runtime` | 7.29.7 | MIT |
| `@floating-ui/core` | 1.7.5 | MIT |
| `@floating-ui/dom` | 1.7.6 | MIT |
| `@floating-ui/react-dom` | 2.1.8 | MIT |
| `@floating-ui/utils` | 0.2.11 | MIT |
| `@hookform/resolvers` | 5.9.2 | MIT |
| `@radix-ui/number` | 1.1.2 | MIT |
| `@radix-ui/primitive` | 1.1.4 | MIT |
| `@radix-ui/react-alert-dialog` | 1.1.16 | MIT |
| `@radix-ui/react-arrow` | 1.1.9 | MIT |
| `@radix-ui/react-collection` | 1.1.9 | MIT |
| `@radix-ui/react-compose-refs` | 1.1.3 | MIT |
| `@radix-ui/react-context` | 1.1.4 | MIT |
| `@radix-ui/react-dialog` | 1.1.16 | MIT |
| `@radix-ui/react-direction` | 1.1.2 | MIT |
| `@radix-ui/react-dismissable-layer` | 1.1.12 | MIT |
| `@radix-ui/react-dropdown-menu` | 2.1.17 | MIT |
| `@radix-ui/react-focus-guards` | 1.1.4 | MIT |
| `@radix-ui/react-focus-scope` | 1.1.9 | MIT |
| `@radix-ui/react-id` | 1.1.2 | MIT |
| `@radix-ui/react-label` | 2.1.9 | MIT |
| `@radix-ui/react-menu` | 2.1.17 | MIT |
| `@radix-ui/react-popper` | 1.3.0 | MIT |
| `@radix-ui/react-portal` | 1.1.11 | MIT |
| `@radix-ui/react-presence` | 1.1.6 | MIT |
| `@radix-ui/react-primitive` | 2.1.5 | MIT |
| `@radix-ui/react-roving-focus` | 1.1.12 | MIT |
| `@radix-ui/react-select` | 2.3.0 | MIT |
| `@radix-ui/react-separator` | 1.1.9 | MIT |
| `@radix-ui/react-slot` | 1.2.5 | MIT |
| `@radix-ui/react-tabs` | 1.1.14 | MIT |
| `@radix-ui/react-tooltip` | 1.2.9 | MIT |
| `@radix-ui/react-use-callback-ref` | 1.1.2 | MIT |
| `@radix-ui/react-use-controllable-state` | 1.2.3 | MIT |
| `@radix-ui/react-use-effect-event` | 0.0.3 | MIT |
| `@radix-ui/react-use-escape-keydown` | 1.1.2 | MIT |
| `@radix-ui/react-use-layout-effect` | 1.1.2 | MIT |
| `@radix-ui/react-use-previous` | 1.1.2 | MIT |
| `@radix-ui/react-use-rect` | 1.1.2 | MIT |
| `@radix-ui/react-use-size` | 1.1.2 | MIT |
| `@radix-ui/react-visually-hidden` | 1.2.5 | MIT |
| `@radix-ui/rect` | 1.1.2 | MIT |
| `@standard-schema/utils` | 0.3.0 | MIT |
| `@tanstack/query-core` | 5.101.0 | MIT |
| `@tanstack/react-query` | 5.101.0 | MIT |
| `@tanstack/react-table` | 8.21.3 | MIT |
| `@tanstack/table-core` | 8.21.3 | MIT |
| `@types/d3-array` | 3.2.2 | MIT |
| `@types/d3-color` | 3.1.3 | MIT |
| `@types/d3-ease` | 3.0.2 | MIT |
| `@types/d3-interpolate` | 3.0.4 | MIT |
| `@types/d3-path` | 3.1.1 | MIT |
| `@types/d3-scale` | 4.0.9 | MIT |
| `@types/d3-shape` | 3.1.8 | MIT |
| `@types/d3-time` | 3.0.4 | MIT |
| `@types/d3-timer` | 3.0.2 | MIT |
| `@types/prop-types` | 15.7.15 | MIT |
| `@types/react` | 18.3.29 | MIT |
| `@types/react-dom` | 18.3.7 | MIT |
| `aria-hidden` | 1.2.6 | MIT |
| `class-variance-authority` | 0.7.1 | Apache-2.0 |
| `clsx` | 2.1.1 | MIT |
| `cookie` | 1.1.1 | MIT |
| `csstype` | 3.2.3 | MIT |
| `d3-array` | 3.2.4 | ISC |
| `d3-color` | 3.1.0 | ISC |
| `d3-ease` | 3.0.1 | BSD-3-Clause |
| `d3-format` | 3.1.2 | ISC |
| `d3-interpolate` | 3.0.1 | ISC |
| `d3-path` | 3.1.0 | ISC |
| `d3-scale` | 4.0.2 | ISC |
| `d3-shape` | 3.2.0 | ISC |
| `d3-time` | 3.1.0 | ISC |
| `d3-time-format` | 4.1.0 | ISC |
| `d3-timer` | 3.0.1 | ISC |
| `decimal.js-light` | 2.5.1 | MIT |
| `detect-node-es` | 1.1.0 | MIT |
| `dom-helpers` | 5.2.1 | MIT |
| `eventemitter3` | 4.0.7 | MIT |
| `fast-equals` | 5.4.0 | MIT |
| `get-nonce` | 1.0.1 | MIT |
| `html-parse-stringify` | 3.0.1 | MIT |
| `i18next` | 26.3.1 | MIT |
| `i18next-browser-languagedetector` | 8.2.1 | MIT |
| `internmap` | 2.0.3 | ISC |
| `js-tokens` | 4.0.0 | MIT |
| `lodash` | 4.18.1 | MIT |
| `loose-envify` | 1.4.0 | MIT |
| `lucide-react` | 0.454.0 | ISC |
| `object-assign` | 4.1.1 | MIT |
| `prop-types` | 15.8.1 | MIT |
| `react` | 18.3.1 | MIT |
| `react-dom` | 18.3.1 | MIT |
| `react-hook-form` | 7.79.0 | MIT |
| `react-i18next` | 17.0.8 | MIT |
| `react-is` | 18.3.1 | MIT |
| `react-remove-scroll` | 2.7.2 | MIT |
| `react-remove-scroll-bar` | 2.3.8 | MIT |
| `react-router` | 7.18.4 | MIT |
| `react-router-dom` | 7.18.4 | MIT |
| `react-smooth` | 4.0.4 | MIT |
| `react-style-singleton` | 2.2.3 | MIT |
| `react-transition-group` | 4.4.5 | BSD-3-Clause |
| `recharts` | 2.15.4 | MIT |
| `recharts-scale` | 0.4.5 | MIT |
| `scheduler` | 0.23.2 | MIT |
| `set-cookie-parser` | 2.7.3 | MIT |
| `sonner` | 1.7.4 | MIT |
| `tailwind-merge` | 2.6.1 | MIT |
| `tiny-invariant` | 1.3.3 | MIT |
| `tslib` | 2.8.1 | 0BSD |
| `typescript` | 5.9.3 | Apache-2.0 |
| `use-callback-ref` | 1.3.3 | MIT |
| `use-sidecar` | 1.1.3 | MIT |
| `use-sync-external-store` | 1.6.0 | MIT |
| `victory-vendor` | 36.9.2 | MIT AND ISC |
| `void-elements` | 3.1.0 | MIT |
| `zod` | 4.6.7 | MIT |

## 3. Пакеты Alpine Linux в образах

Образы `datasafe-storage-server` и `datasafe-console` собираются на базе `alpine:3.24` без дополнительных `apk add`; пакет `wget` отдельно не ставится (healthcheck использует busybox). Состав — по SBOM базового образа:

| Пакет | Версия | Лицензия |
|---|---|---|
| `alpine-baselayout` | 3.7.2-r1 | GPL-2.0-only |
| `alpine-baselayout-data` | 3.7.2-r1 | GPL-2.0-only |
| `alpine-keys` | 2.6-r0 | MIT |
| `alpine-release` | 3.24.3-r0 | MIT |
| `apk-tools` | 3.0.8-r0 | GPL-2.0-only |
| `busybox` | 1.37.0-r31 | GPL-2.0-only |
| `busybox-binsh` | 1.37.0-r31 | GPL-2.0-only |
| `ca-certificates-bundle` | 20261020-r0 | MPL-2.0 AND MIT |
| `libapk` | 3.0.8-r0 | GPL-2.0-only |
| `libcrypto3` | 3.5.9-r0 | Apache-2.0 |
| `libssl3` | 3.5.9-r0 | Apache-2.0 |
| `musl` | 1.2.6-r3 | MIT |
| `musl-utils` | 1.2.6-r3 | MIT AND BSD-2-Clause AND GPL-2.0-or-later |
| `scanelf` | 1.3.9-r1 | GPL-2.0-only |
| `ssl_client` | 1.37.0-r31 | GPL-2.0-only |
| `zlib` | 1.3.2-r1 | Zlib |

Пакеты под GPL-2.0 (busybox, apk-tools и др.) — отдельные программы системы; их исходные тексты доступны в Alpine Linux (<https://git.alpinelinux.org/aports>). Бинарники DataSafeS3 с ними не слинкованы.

## 4. Необязательные внешние компоненты (не входят в образы DataSafeS3)

Эти программы запускаются отдельными контейнерами из образов их авторов и скачиваются из реестров по вашему выбору. Лицензии и условия — их собственные.

| Компонент | Лицензия | Назначение | Примечание |
|---|---|---|---|
| Grafana | AGPL-3.0-only | optional dashboards image/profile; not part of DataSafeS3 images | Отдельный образ grafana/grafana, запускается как отдельный сервис. DataSafeS3 не включает и не изменяет код Grafana. Условия AGPL-3.0 относятся к вашей эксплуатации Grafana. |
| HashiCorp Vault | BUSL-1.1 | optional KMS/transit (compose profile, Helm values) | Отдельный образ hashicorp/vault. Лицензия BUSL-1.1 не является открытой; проверьте допустимость использования в вашей организации. Совместимая открытая альтернатива с тем же API — OpenBao (MPL-2.0), рекомендуется для установок, где BUSL неприемлема. |
| Prometheus | Apache-2.0 | optional metrics scraping (compose, Helm) | Отдельный образ prom/prometheus. |
| PostgreSQL | PostgreSQL License | optional metadata backend (HA mode) | Отдельный образ postgres. |
| Apache Kafka | Apache-2.0 | optional event stream | Отдельный образ; клиент в бинарнике — Go-модуль из списка выше. |
| NATS Server | Apache-2.0 | optional event stream; used in tests only (go.mod) | В релизный бинарник не входит; используется в тестах. |
| Keycloak | Apache-2.0 | optional OIDC/LDAP lab integration | Отдельный образ quay.io/keycloak/keycloak в примерах интеграции. |
| Caddy | Apache-2.0 | optional TLS reverse proxy (legacy chart, compose) | Отдельный образ caddy. |
| OpenLDAP (osixia/openldap or similar) | OpenLDAP Public License | optional LDAP lab | Только в лабораторных стендах. |

Список не заменяет юридическую оценку. Если вам нужны полные тексты, берите их из SBOM релиза и репозиториев модулей.
