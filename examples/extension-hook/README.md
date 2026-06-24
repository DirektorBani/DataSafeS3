# Extension hook sample

Demonstrates `extensions.RegisterObjectUploadValidator` for Community Edition integrators.

## Pattern

1. Copy `main.go` logic into your fork's `main` or a blank-import package.
2. Build storage-server with `-tags=extensions` when hook wiring is enabled in your branch.

## Alternative (no fork)

Use **webhooks** (`object.created`) or Admin API policies for validation without recompiling.

## Events

See `internal/extensions/hooks.go` for documented webhook event types.
