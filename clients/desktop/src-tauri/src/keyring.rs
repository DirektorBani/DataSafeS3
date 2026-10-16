/// OS credential store. Entries are `datasafe-desktop/<profile_id>/<kind>`.
/// A missing Secret Service does not fail the app: the caller keeps the
/// secret in memory for this process only.
pub struct SecretStore;

const SERVICE: &str = "datasafe-desktop";
const KINDS: &[&str] = &["password", "token", "api_token", "access_secret", "expires_at"];

impl SecretStore {
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
