# Compose lab env templates

Copy a template to the **repository root** before `docker compose --env-file …` (root `.env.*` working copies are gitignored):

```bash
cp deploy/compose/env/.env.ha.example .env.ha
cp deploy/compose/env/.env.erasure.example .env.erasure
cp deploy/compose/env/.env.site-a.example .env.site-a
cp deploy/compose/env/.env.site-b.example .env.site-b
cp deploy/compose/env/.env.vault.example .env.vault
cp deploy/compose/env/.env.security-test.example .env.security-test
```

Do not commit filled `.env.ha` / `.env.site-*` files — they may contain local secrets and machine paths.
