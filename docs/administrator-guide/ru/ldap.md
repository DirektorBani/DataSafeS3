**[English](../en/ldap.md)** | Русский

# Интеграция LDAP

Синхронизация пользователей и сопоставление LDAP-групп с ролями DataSafeS3 при входе.

## Настройка

**Admin → Settings → System → LDAP**

| Поле | Описание |
|------|----------|
| URL | `ldap://host:389` или `ldaps://` |
| Bind DN | Сервисная учётная запись |
| Base DN | База поиска пользователей |
| Сопоставление групп | LDAP-группа → `administrator` / `operator` / `user` |

## Тест и синхронизация

Кнопки **Проверить подключение** и **Синхронизировать** в интерфейсе.

## Тестовое окружение

[LDAP + Keycloak standalone](../../ru/integrations/ldap-keycloak-standalone.md)

## Полное руководство

[Руководство пользователя — LDAP и SSO](../../ru/user-guide/README.md)
