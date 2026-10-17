use std::fs;
use std::path::{Path, PathBuf};
use std::sync::Mutex;

use serde_json::{json, Value};

use crate::profiles;

static SAVE: Mutex<()> = Mutex::new(());

/// Language and theme. The file sits next to profiles.json, so uninstall
/// keeps it. The WebView2 profile does not.
#[derive(Clone, Debug, PartialEq, Eq)]
struct Prefs {
    locale: String,
    theme: String,
}

fn settings_path() -> PathBuf {
    profiles::data_dir().join("settings.json")
}

pub fn load() -> Value {
    let prefs = read_file(&settings_path());
    apply_uia_fixture(
        json!({
            "locale": prefs.locale,
            "theme": prefs.theme,
        }),
        std::env::var("DATASAFE_UIA_FIXTURE").ok().as_deref() == Some("1"),
    )
}

/// The Windows smoke starts the installed app with DATASAFE_UIA_FIXTURE=1.
/// The flag is not written to settings.json.
pub fn apply_uia_fixture(mut value: Value, enabled: bool) -> Value {
    if enabled {
        if let Some(obj) = value.as_object_mut() {
            obj.insert("uia_fixture".to_string(), Value::Bool(true));
        }
    }
    value
}

pub fn save(locale: Option<&str>, theme: Option<&str>) -> Result<(), String> {
    let _guard = SAVE.lock().map_err(|err| err.to_string())?;
    let path = settings_path();
    write_file(&path, locale, theme)
}

fn read_file(path: &Path) -> Prefs {
    let raw = fs::read(path).unwrap_or_default();
    let parsed: Value = serde_json::from_slice(&raw).unwrap_or(Value::Null);
    Prefs {
        locale: known_locale(text(&parsed, "locale")),
        theme: known_theme(text(&parsed, "theme")),
    }
}

fn write_file(path: &Path, locale: Option<&str>, theme: Option<&str>) -> Result<(), String> {
    let current = read_file(path);
    let mut next = current;
    if let Some(value) = locale {
        let value = known_locale(value);
        if !value.is_empty() {
            next.locale = value;
        }
    }
    if let Some(value) = theme {
        let value = known_theme(value);
        if !value.is_empty() {
            next.theme = value;
        }
    }
    if let Some(dir) = path.parent() {
        fs::create_dir_all(dir).map_err(|err| err.to_string())?;
    }
    let body = json!({ "locale": next.locale, "theme": next.theme });
    let raw = serde_json::to_vec_pretty(&body).map_err(|err| err.to_string())?;
    let tmp = path.with_extension("json.tmp");
    fs::write(&tmp, raw).map_err(|err| err.to_string())?;
    if fs::rename(&tmp, path).is_err() {
        let _ = fs::remove_file(path);
        fs::rename(&tmp, path).map_err(|err| err.to_string())?;
    }
    Ok(())
}

fn text<'a>(value: &'a Value, key: &str) -> &'a str {
    value.get(key).and_then(Value::as_str).unwrap_or("")
}

fn known_locale(value: &str) -> String {
    match value {
        "en" | "ru" => value.to_string(),
        _ => String::new(),
    }
}

fn known_theme(value: &str) -> String {
    match value {
        "light" | "dark" | "system" => value.to_string(),
        _ => String::new(),
    }
}

#[cfg(test)]
mod tests {
    use super::apply_uia_fixture;
    use super::read_file;
    use super::write_file;
    use serde_json::json;

    #[test]
    fn settings_file_keeps_each_field() {
        let dir = std::env::temp_dir().join(format!("datasafe-settings-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        let path = dir.join("settings.json");
        write_file(&path, Some("ru"), None).unwrap();
        write_file(&path, None, Some("light")).unwrap();
        write_file(&path, Some("nope"), Some("neon")).unwrap();
        let prefs = read_file(&path);
        assert_eq!(prefs.locale, "ru");
        assert_eq!(prefs.theme, "light");
        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn uia_fixture_is_only_added_when_asked() {
        let plain = json!({"locale": "en", "theme": "dark"});
        assert!(apply_uia_fixture(plain.clone(), false).get("uia_fixture").is_none());
        assert_eq!(
            apply_uia_fixture(plain, true).get("uia_fixture").and_then(|value| value.as_bool()),
            Some(true)
        );
    }
}
