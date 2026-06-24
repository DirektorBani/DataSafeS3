English | **[Русский](../../ru/user-guide/04-bezopasnost-i-profil.md)**

# 4. Security and Profile

[← Keys and quotas](03-keys-and-quotas.md) | [Table of contents](README.md) | Next: [Administration →](05-administration.md)

---

## Profile

The **Profile** menu contains your account settings:

- password change (if available);
- **multi-factor authentication (MFA)**;
- recovery codes.

![Profile section — MFA setup](../../user-guide/images/mfa-profile.png)

---

## MFA — Multi-Factor Authentication

MFA adds a second step at sign-in: besides the password, you need a one-time code from your phone.

Recommended app: **Google Authenticator** (Authy, Microsoft Authenticator, and any TOTP app also work).

### Enable MFA

1. Sign in to the console → **Profile**.
2. Click **Enable MFA**.
3. A **QR code** appears on screen.
4. Open Google Authenticator → **Add** → **Scan QR code**.
5. Enter the **6 digits** from the app in the confirmation field on the Profile page.
6. Save **recovery codes** — click Copy or Download.  
   You will need them if you lose your phone.

### Sign In with MFA

1. Enter login and password as usual.
2. A second screen opens.
3. Enter the current **6 digits** from Authenticator.
4. Click confirm.

The code changes every ~30 seconds — if you miss it, wait for the next one.

### Disable MFA

1. **Profile** → **Disable MFA**.
2. Enter your password and a code from the app (or a recovery code).

### Recovery Codes

- One-time codes if you lose your phone.
- Store them separately from your password (paper, safe, password manager).
- Each code is consumed after use.

### MFA Requirement for Administrators

An administrator can enable **Require MFA for administrators** in **Settings → MFA** — then all admins must configure MFA.

---

## Security Tips

| Tip | Why |
|-----|-----|
| Change the `admin` password after installation | Protection against compromise |
| Enable MFA | Even with a leaked password, sign-in without the phone is impossible |
| Do not share Secret Key and API tokens | Full access to data |
| Use HTTPS on production servers | Encrypted traffic |

---

## What's Next?

- [Administration →](05-administration.md)
