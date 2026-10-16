# `/api/v1` contract

`v1.yaml` is the canonical OpenAPI **3.1** contract for the operations DataSafeS3 Desktop calls, plus the community integration paths that already lived in `internal/openapi/openapi.yaml`.

`make openapi` copies this file to:

- `internal/openapi/openapi.yaml` (embedded and served at `/api/v1/openapi.yaml`)
- `docs/api/openapi.yaml`

## 3.1 subset

`oasdiff` v1.15.0 (kin-openapi v0.136) parses this document. The subset that gate understands:

- `nullable: true` is kept
- `enum` values are strings; a field that can be null uses `nullable` instead of `enum: [..., null]`
- path templates use `{param}`, including `{key}` where the server route is `{key...}`

Compatibility rules: [docs/api/compatibility.md](../../docs/api/compatibility.md).
