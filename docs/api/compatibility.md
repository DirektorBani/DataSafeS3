# API compatibility / Совместимость API

Canonical contract: [`api/openapi/v1.yaml`](../../api/openapi/v1.yaml). Server 1.6.1 publishes it. `info.version` in that file is the spec version (`1.6.1`). `GET /healthz` and `capabilities.server_version` use `HeartbeatVersion`: an unstamped or pre-1.6.0 build still advertises `1.6.0`, and a build stamped `1.6.1` advertises `1.6.1`.

## Window / Окно

- DataSafeS3 1.x keeps `/api/v1` compatible with the current Desktop release and the two previous minor Desktop releases once those releases exist.
- Desktop 1.x talks to servers `>= 1.5.0`. A server older than 1.5.0 is rejected.
- Kafka and Vault stay optional. A single node with Bolt, and with `STORAGE_KMS_*` unset, does not open a broker or a KMS connection.

## Additive changes / Аддитивные изменения

Allowed inside `/api/v1` without a new major version:

- a new operation or an optional request field
- an optional response field, a new enum value in a response, a new error `code`
- a new `features` key

Not allowed inside `/api/v1`:

- removing or renaming a field, operation, or error code
- changing the type or the meaning of an existing field
- making an optional field required

A security fix may tighten a check in a minor release. If an old client stops working, set `clients.desktop.min_required` and `min_required_reason: security`. Any other break needs the pull-request label `api-breaking-approved` and a release note. Removal waits for `/api/v2`.

The server ignores unknown JSON request fields. Clients ignore unknown response fields and unknown error codes, and fall back to the HTTP status.

## Capabilities

`GET /api/v1/capabilities` is unauthenticated for `api`, `server_version`, `spec_version`, and `login_methods` only. `features`, `clients.desktop`, and `deprecations` are returned only to a valid bearer token. A missing `Authorization` header is the public document. An invalid token is `401`, not the public document.

`Cache-Control` is `public, max-age=300` for the public body and `private, max-age=300` for the authenticated body, so a shared cache does not store the feature list.

`features` omits a key the process cannot do, including a feature turned off by configuration. `clients.desktop.max_tested` stays null until a Desktop build has passed this server's contract suite. `min_required` stays null until a security floor is set.

The body never includes an IdP address, a username, or a secret. `GET /api/v1/auth/oidc/config` still returns the issuer when OIDC is enabled; capabilities does not.

## Deprecation / Устаревание

A deprecated operation sets `Deprecation: @<unix>`, `Sunset` (HTTP-date), and `Link: <url>; rel="deprecation"` from the OpenAPI extensions `x-deprecated-at`, `x-sunset`, and `x-notes-url`. 1.6.1 does not mark any production operation deprecated. Deletion happens only in `/api/v2`, after `Sunset`.

## Errors / Ошибки

Error JSON keeps the 1.6.0 `error` string and adds `code`, `message`, `details`, and `request_id`. `code` is stable. `429` responses include `Retry-After`. S3 XML errors are unchanged.

| `code` | When |
| --- | --- |
| `auth.unauthorized` | missing or invalid bearer token |
| `auth.invalid_credentials` | password rejected |
| `auth.session_expired` | OIDC session is no longer active |
| `auth.step_up_required` | the action needs a step-up token |
| `auth.mfa_setup_required` | an administrator must enroll MFA |
| `auth.mfa_required` | MFA is required |
| `auth.mfa_token_invalid` | MFA or exchange token was rejected |
| `access.forbidden` | the caller lacks permission |
| `access.quota_exceeded` | a quota blocked the write |
| `sharing.policy_denied` | share or presign policy refused the call |
| `resource.not_found` | the resource does not exist |
| `resource.conflict` | the resource already exists or is not empty |
| `resource.gone` | the resource expired |
| `multipart.upload_not_found` | `uploadId` is not bound to this bucket |
| `request.invalid` | the body or a parameter failed validation |
| `rate_limited` | too many requests |
| `setup.required` | first-run setup is not finished |
| `setup.password_change_required` | setup cannot finish until the admin password changes |
| `internal` | the server failed |

Clients that do not know a code use the HTTP status. `GET /me` does not return `403`: that route is exempt from the MFA-setup gate.
