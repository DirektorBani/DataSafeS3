/// OS credential store. Entries are `datasafe-desktop/<profile_id>/<kind>`.
/// The build must link the native store. The in-memory mock reports success
/// and then drops the secret, including inside one process.
/// A missing Secret Service does not fail the app: the caller keeps the
/// secret in memory for this process only.
pub struct SecretStore;

const SERVICE: &str = "datasafe-desktop";
const KINDS: &[&str] = &["password", "token", "api_token", "access_secret", "expires_at"];

impl SecretStore {
    /// Fail when the active store is the in-memory mock.
    pub fn require_native() -> Result<(), String> {
        let builder = linked_native_builder();
        match builder.persistence() {
            keyring::credential::CredentialPersistence::EntryOnly
            | keyring::credential::CredentialPersistence::ProcessOnly => {
                return Err("OS credential store does not persist".into());
            }
            _ => {}
        }
        let entry = entry("backend-check", "password")?;
        if entry
            .get_credential()
            .downcast_ref::<keyring::mock::MockCredential>()
            .is_some()
        {
            return Err("OS credential store is the in-memory mock".into());
        }
        Ok(())
    }

    pub fn set(&self, profile_id: &str, kind: &str, value: &str) -> Result<(), String> {
        let entry = entry(profile_id, kind)?;
        entry.set_password(value).map_err(|err| err.to_string())
    }

    pub fn get(&self, profile_id: &str, kind: &str) -> Result<Option<String>, String> {
        let entry = entry(profile_id, kind)?;
        match entry.get_password() {
            Ok(value) => Ok(Some(value)),
            Err(err) if is_missing(&err) => Ok(None),
            Err(err) => Err(err.to_string()),
        }
    }

    pub fn delete(&self, profile_id: &str, kind: &str) -> Result<(), String> {
        let entry = entry(profile_id, kind)?;
        match entry.delete_credential() {
            Ok(()) => Ok(()),
            Err(err) if is_missing(&err) => Ok(()),
            Err(err) => Err(err.to_string()),
        }
    }

    pub fn delete_profile(&self, profile_id: &str) -> Result<(), String> {
        for kind in KINDS {
            self.delete(profile_id, kind)?;
        }
        Ok(())
    }
}

fn entry(profile_id: &str, kind: &str) -> Result<keyring::Entry, String> {
    keyring::Entry::new(SERVICE, &format!("{profile_id}/{kind}")).map_err(|err| err.to_string())
}

fn is_missing(err: &keyring::Error) -> bool {
    matches!(err, keyring::Error::NoEntry)
}

/// The platform module exists only when that keyring feature is enabled.
/// A build that forgot the feature does not compile.
fn linked_native_builder() -> Box<keyring::CredentialBuilder> {
    #[cfg(target_os = "windows")]
    {
        keyring::windows::default_credential_builder()
    }
    #[cfg(any(target_os = "linux", target_os = "freebsd", target_os = "openbsd"))]
    {
        keyring::secret_service::default_credential_builder()
    }
    #[cfg(target_os = "macos")]
    {
        keyring::macos::default_credential_builder()
    }
    #[cfg(target_os = "ios")]
    {
        keyring::ios::default_credential_builder()
    }
    #[cfg(not(any(
        target_os = "windows",
        target_os = "linux",
        target_os = "freebsd",
        target_os = "openbsd",
        target_os = "macos",
        target_os = "ios",
    )))]
    {
        compile_error!("DataSafeS3 Desktop has no OS credential store for this target")
    }
}

#[cfg(test)]
mod tests {
    use super::SecretStore;

    #[test]
    fn native_store_is_not_the_mock_and_forget_deletes() {
        SecretStore::require_native().expect("mock credential store is active");
        let id = format!("forget-probe-{}", std::process::id());
        let store = SecretStore;
        let _ = store.delete_profile(&id);
        match store.set(&id, "password", "remembered") {
            Ok(()) => {
                let saved = store
                    .get(&id, "password")
                    .expect("read")
                    .expect("a new entry must see the password the previous entry saved");
                assert_eq!(saved, "remembered");
                store.set(&id, "token", "jwt").expect("token");
                store.delete_profile(&id).expect("forget");
                assert!(store.get(&id, "password").expect("password after forget").is_none());
                assert!(store.get(&id, "token").expect("token after forget").is_none());
            }
            Err(err) => {
                // The native backend is linked. CI and a machine without a
                // Secret Service daemon cannot open a collection. That error
                // is not the mock, which would have stored the password.
                let lower = err.to_ascii_lowercase();
                assert!(
                    lower.contains("platform secure storage")
                        || lower.contains("org.freedesktop.secrets")
                        || lower.contains("dbus"),
                    "unexpected store error: {err}"
                );
            }
        }
    }
}
