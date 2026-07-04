English | **[Русский](../ru/teams.md)**

# Teams

Teams let administrators group users for console ownership, assignment, and future collaboration workflows.

## Console

Open **Admin → Teams** to:

1. Create a team with a display name.
2. Add or remove members.
3. Assign users to a team from the user management flow.

## API

The Admin API exposes:

| Operation | Path |
|-----------|------|
| List teams | `GET /api/v1/teams` |
| Create team | `POST /api/v1/teams` |
| Update team | `PUT /api/v1/teams/{id}` |
| Delete team | `DELETE /api/v1/teams/{id}` |
| Manage members | `GET/POST/DELETE /api/v1/teams/{id}/members` |

All routes require an administrator JWT. See `docs/api/openapi-full.yaml` for schemas.
