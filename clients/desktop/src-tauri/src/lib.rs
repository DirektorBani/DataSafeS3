mod agent;
mod keyring;
mod logging;
mod profiles;
mod settings;

use agent::Agent;
use keyring::SecretStore;
use serde_json::{json, Value};
use std::collections::HashMap;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Mutex;
use tauri::{Manager, State};

mod tray;

struct MemorySession {
    token: String,
    mfa_token: String,
}

pub(crate) struct Desktop {
    pub(crate) agent: Mutex<Agent>,
    secrets: SecretStore,
    memory: Mutex<HashMap<String, MemorySession>>,
}

struct CloseHides(AtomicBool);

fn call(state: &Desktop, method: &str, params: Value) -> Result<Value, String> {
    let result = state.agent.lock().map_err(|err| err.to_string())?.call(method, params);
    match &result {
        Ok(_) if method == "transfer.list" => {}
        Ok(_) => logging::info(method),
        Err(err) => logging::error(&format!("{method} {err}")),
    }
    result
}

fn session_mut(state: &Desktop) -> Result<std::sync::MutexGuard<'_, HashMap<String, MemorySession>>, String> {
    state.memory.lock().map_err(|err| err.to_string())
}

fn remember_token(state: &Desktop, id: &str, token: &str) -> bool {
    if token.is_empty() {
        return true;
    }
    if let Ok(mut memory) = session_mut(state) {
        let entry = memory.entry(id.to_string()).or_insert(MemorySession {
            token: String::new(),
            mfa_token: String::new(),
        });
        entry.token = token.to_string();
    }
    state.secrets.set(id, "token", token).is_ok()
}

fn token_for(state: &Desktop, id: &str) -> String {
    if let Ok(memory) = state.memory.lock() {
        if let Some(session) = memory.get(id) {
            if !session.token.is_empty() {
                return session.token.clone();
            }
        }
    }
    state.secrets.get(id, "token").ok().flatten().unwrap_or_default()
}

fn take_secrets(value: &mut Value) -> (String, String) {
    let Some(obj) = value.as_object_mut() else {
        return (String::new(), String::new());
    };
    let token = obj.remove("token").and_then(|item| item.as_str().map(str::to_string)).unwrap_or_default();
    let mfa = obj.remove("mfa_token").and_then(|item| item.as_str().map(str::to_string)).unwrap_or_default();
    (token, mfa)
}

fn persisted_login_status(status: &str) -> bool {
    matches!(status, "signed_in" | "mfa_required")
}

fn present_login(state: &Desktop, id: &str, result: Value, password: &str, remember: bool) -> Result<Value, String> {
    let status = result.get("status").and_then(|item| item.as_str()).unwrap_or("").to_string();
    if !persisted_login_status(&status) {
        let mut view = result;
        if let Some(obj) = view.as_object_mut() {
            obj.remove("token");
            obj.remove("mfa_token");
            obj.insert("secrets_persisted".into(), Value::Bool(false));
        }
        return Ok(view);
    }
    let mut view = store_login(state, id, result, status == "signed_in")?;
    if !remember_password(state, id, password, remember) {
        if let Some(obj) = view.as_object_mut() {
            obj.insert("secrets_persisted".into(), Value::Bool(false));
        }
    }
    Ok(view)
}

fn remember_password(state: &Desktop, id: &str, password: &str, remember: bool) -> bool {
    if remember {
        return state.secrets.set(id, "password", password).is_ok();
    }
    let _ = state.secrets.delete(id, "password");
    true
}

fn store_login(state: &Desktop, id: &str, mut result: Value, persist_token: bool) -> Result<Value, String> {
    let (token, mfa) = take_secrets(&mut result);
    if !mfa.is_empty() {
        if let Ok(mut memory) = session_mut(state) {
            let entry = memory.entry(id.to_string()).or_insert(MemorySession {
                token: String::new(),
                mfa_token: String::new(),
            });
            entry.mfa_token = mfa;
        }
    }
    let persisted = if token.is_empty() || !persist_token {
        true
    } else {
        remember_token(state, id, &token)
    };
    if !token.is_empty() && !persist_token {
        if let Ok(mut memory) = session_mut(state) {
            let entry = memory.entry(id.to_string()).or_insert(MemorySession {
                token: String::new(),
                mfa_token: String::new(),
            });
            entry.token = token;
        }
    }
    if let Some(obj) = result.as_object_mut() {
        obj.insert("secrets_persisted".into(), Value::Bool(persisted));
    }
    Ok(result)
}

#[tauri::command]
fn profile_list(state: State<Desktop>) -> Result<Value, String> {
    let mut result = call(&state, "profile.list", json!({}))?;
    if let Some(items) = result.get_mut("profiles").and_then(Value::as_array_mut) {
        for item in items {
            let id = item.get("id").and_then(Value::as_str).unwrap_or("").to_string();
            let session = item.get("session").and_then(Value::as_str).unwrap_or("");
            if session == "signed_in" && token_for(&state, &id).is_empty() {
                if let Some(obj) = item.as_object_mut() {
                    obj.insert("session".into(), Value::String("reauth".into()));
                }
            }
        }
    }
    Ok(result)
}

#[tauri::command]
fn profile_save(state: State<Desktop>, profile: Value) -> Result<Value, String> {
    call(&state, "profile.put", json!({ "profile": profile }))
}

#[tauri::command]
fn profile_forget(state: State<Desktop>, id: String) -> Result<Value, String> {
    // Credential entries go first. A missing entry is success. If the OS
    // store cannot be opened, nothing was persisted there; the profile
    // still goes. Any other delete error leaves the profile so Forget can
    // be retried.
    if let Err(err) = state.secrets.delete_profile(&id) {
        if !err.contains("Couldn't access platform secure storage") {
            return Err(err);
        }
    }
    if let Ok(mut memory) = session_mut(&state) {
        memory.remove(&id);
    }
    call(&state, "profile.delete", json!({ "id": id }))
}

fn browse_method_allowed(method: &str) -> bool {
    matches!(
        method,
        "account.me"
            | "bucket.list"
            | "bucket.create"
            | "bucket.delete"
            | "bucket.settings"
            | "bucket.encryption"
            | "object.list"
            | "object.meta"
            | "object.meta.put"
            | "object.tags"
            | "object.tags.put"
            | "object.retention"
            | "object.versions"
            | "object.restore"
            | "object.rename"
            | "object.delete"
            | "folder.create"
            | "folder.delete"
            | "trash.list"
            | "trash.restore"
            | "trash.purge"
            | "share.list"
            | "share.create"
            | "share.revoke"
            | "share.revoke_all"
            | "share.policy"
            | "presign.create"
    )
}

#[tauri::command]
fn browse(state: State<Desktop>, id: String, method: String, params: Option<Value>) -> Result<Value, String> {
    if !browse_method_allowed(&method) {
        return Err("method is not a browse call".into());
    }
    let token = token_for(&state, &id);
    let mut body = params.unwrap_or_else(|| json!({}));
    if let Some(obj) = body.as_object_mut() {
        obj.remove("token");
        obj.remove("password");
        obj.insert("id".into(), Value::String(id));
        obj.insert("token".into(), Value::String(token));
    } else {
        return Err("browse params must be an object".into());
    }
    let mut result = call(&state, &method, body)?;
    strip_secrets(&mut result);
    Ok(result)
}

fn transfer_method_allowed(method: &str) -> bool {
    matches!(
        method,
        "transfer.list" | "transfer.enqueue" | "transfer.cancel" | "transfer.resolve" | "transfer.files" | "transfer.resume" | "transfer.retry"
    )
}

fn strip_secrets(value: &mut Value) {
    match value {
        Value::Object(map) => {
            map.remove("token");
            map.remove("password");
            map.remove("access_secret");
            map.remove("mfa_token");
            map.remove("step_up_code");
            map.remove("totp_code");
            map.remove("wrapped_dek");
            for child in map.values_mut() {
                strip_secrets(child);
            }
        }
        Value::Array(items) => {
            for child in items {
                strip_secrets(child);
            }
        }
        _ => {}
    }
}

#[tauri::command]
fn transfer(state: State<Desktop>, id: String, method: String, params: Option<Value>) -> Result<Value, String> {
    if !transfer_method_allowed(&method) {
        return Err("method is not a transfer call".into());
    }
    let token = token_for(&state, &id);
    let mut body = params.unwrap_or_else(|| json!({}));
    let obj = body.as_object_mut().ok_or("transfer params must be an object")?;
    obj.remove("token");
    obj.remove("password");
    obj.remove("access_secret");
    obj.insert("id".into(), Value::String(id.clone()));
    obj.insert("token".into(), Value::String(token));
    if method == "transfer.enqueue" || method == "transfer.resume" || method == "transfer.retry" || method == "transfer.cancel" {
        if let Ok(Some(secret)) = state.secrets.get(&id, "access_secret") {
            if !secret.is_empty() {
                obj.insert("access_secret".into(), Value::String(secret));
            }
        }
    }
    let mut result = call(&state, &method, body)?;
    strip_secrets(&mut result);
    Ok(result)
}

fn diag_method_allowed(method: &str) -> bool {
    matches!(method, "diag.preview" | "diag.export")
}

#[tauri::command]
fn diag(state: State<Desktop>, method: String, params: Option<Value>) -> Result<Value, String> {
    if !diag_method_allowed(&method) {
        return Err("method is not a diagnostics call".into());
    }
    let mut body = params.unwrap_or_else(|| json!({}));
    if let Some(obj) = body.as_object_mut() {
        obj.remove("token");
        obj.remove("password");
        obj.remove("access_secret");
    } else {
        return Err("diagnostics params must be an object".into());
    }
    let mut result = call(&state, &method, body)?;
    strip_secrets(&mut result);
    Ok(result)
}

#[tauri::command]
fn server_info(state: State<Desktop>, id: String) -> Result<Value, String> {
    let token = token_for(&state, &id);
    call(&state, "server.info", json!({ "id": id, "token": token }))
}

#[tauri::command]
fn settings_load() -> Result<Value, String> {
    Ok(settings::load())
}

#[tauri::command]
fn settings_save(locale: Option<String>, theme: Option<String>) -> Result<(), String> {
    settings::save(locale.as_deref(), theme.as_deref())
}

#[tauri::command]
fn auth_login(state: State<Desktop>, id: String, username: String, password: String, remember: bool) -> Result<Value, String> {
    let password = if password.is_empty() {
        state.secrets.get(&id, "password")?.filter(|value| !value.is_empty()).ok_or("password is required")?
    } else {
        password
    };
    let result = call(&state, "auth.login", json!({ "id": id, "username": username, "password": password }))?;
    present_login(&state, &id, result, &password, remember)
}

#[tauri::command]
fn auth_mfa(state: State<Desktop>, id: String, code: String) -> Result<Value, String> {
    let mfa_token = session_mut(&state)?.get(&id).map(|session| session.mfa_token.clone()).unwrap_or_default();
    if mfa_token.is_empty() {
        return Err("sign in again before the verification code".into());
    }
    let result = call(&state, "auth.mfa", json!({ "id": id, "mfa_token": mfa_token, "mfa_code": code }))?;
    if let Ok(mut memory) = session_mut(&state) {
        if let Some(session) = memory.get_mut(&id) {
            session.mfa_token.clear();
        }
    }
    store_login(&state, &id, result, true)
}

#[tauri::command]
fn auth_logout(state: State<Desktop>, id: String) -> Result<Value, String> {
    let token = token_for(&state, &id);
    let result = call(&state, "auth.logout", json!({ "id": id, "token": token }))?;
    let _ = state.secrets.delete(&id, "token");
    if let Ok(mut memory) = session_mut(&state) {
        memory.remove(&id);
    }
    Ok(result)
}

#[tauri::command]
fn auth_oidc_password(state: State<Desktop>, id: String, username: String, password: String, remember: bool) -> Result<Value, String> {
    let result = call(&state, "auth.oidc", json!({ "id": id, "username": username, "password": password }))?;
    present_login(&state, &id, result, &password, remember)
}

#[tauri::command]
fn auth_oidc_exchange(state: State<Desktop>, id: String, exchange_code: String) -> Result<Value, String> {
    let result = call(&state, "auth.oidc_exchange", json!({ "id": id, "exchange_code": exchange_code }))?;
    store_login(&state, &id, result, true)
}

#[tauri::command]
fn auth_api_token(state: State<Desktop>, id: String, token: String) -> Result<Value, String> {
    let saved = state.secrets.set(&id, "api_token", &token).is_ok();
    if let Ok(mut memory) = session_mut(&state) {
        memory.insert(id.clone(), MemorySession { token: token.clone(), mfa_token: String::new() });
    }
    let result = call(&state, "auth.token", json!({ "id": id, "token": token }))?;
    let mut view = store_login(&state, &id, result, true)?;
    if let Some(obj) = view.as_object_mut() {
        obj.insert("secrets_persisted".into(), Value::Bool(saved));
    }
    Ok(view)
}

#[tauri::command]
fn auth_access_key(state: State<Desktop>, id: String, secret: String) -> Result<Value, String> {
    let saved = state.secrets.set(&id, "access_secret", &secret).is_ok();
    let info = call(&state, "server.info", json!({ "id": id }))?;
    // Sign-in for an access key does not go through auth.login. Resume here,
    // in the command that stored the secret, so a paused transfer continues
    // without waiting for the window to refresh the profile list.
    let _ = call(&state, "transfer.resume", json!({ "id": id, "access_secret": secret }));
    Ok(json!({
        "status": "credentials_saved",
        "secrets_persisted": saved,
        "server": info.get("server").cloned().unwrap_or(Value::Null),
    }))
}

fn profile_server_url(state: &Desktop, id: &str) -> Result<String, String> {
    let listed = call(state, "profile.list", json!({}))?;
    let items = listed.get("profiles").and_then(Value::as_array).ok_or("profile was not found")?;
    for item in items {
        if item.get("id").and_then(Value::as_str) != Some(id) {
            continue;
        }
        let url = item.get("server_url").and_then(Value::as_str).unwrap_or("").trim().to_string();
        if url.is_empty() {
            return Err("server url is required".into());
        }
        return Ok(url);
    }
    Err("profile was not found".into())
}

fn oidc_login_url(server: &str) -> Result<String, String> {
    let base = server.trim().trim_end_matches('/');
    if !(base.starts_with("https://") || base.starts_with("http://")) {
        return Err("server url is not http(s)".into());
    }
    Ok(format!("{base}/api/v1/auth/oidc/login"))
}

fn open_browser(url: &str) -> Result<(), String> {
    #[cfg(target_os = "windows")]
    {
        return std::process::Command::new("rundll32.exe")
            .args(["url.dll,FileProtocolHandler", url])
            .spawn()
            .map(|_| ())
            .map_err(|err| err.to_string());
    }
    #[cfg(target_os = "linux")]
    {
        return std::process::Command::new("xdg-open")
            .arg(url)
            .spawn()
            .map(|_| ())
            .map_err(|err| err.to_string());
    }
    #[cfg(not(any(target_os = "windows", target_os = "linux")))]
    {
        let _ = url;
        Err("this desktop build does not open a browser on this system".into())
    }
}

#[tauri::command]
fn auth_oidc_browser(state: State<Desktop>, id: String) -> Result<(), String> {
    let server = profile_server_url(&state, &id)?;
    open_browser(&oidc_login_url(&server)?)
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .setup(|app| {
            SecretStore::require_native()?;
            let dir = profiles::data_dir();
            let _ = std::fs::create_dir_all(&dir);
            logging::init(&dir.join("logs"));
            let resources = app.path().resource_dir().ok();
            app.manage(Desktop {
                agent: Mutex::new(Agent::open(dir, resources)),
                secrets: SecretStore,
                memory: Mutex::new(HashMap::new()),
            });
            let hides = tray::install(app);
            app.manage(CloseHides(AtomicBool::new(hides)));
            Ok(())
        })
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                let hide = window
                    .app_handle()
                    .try_state::<CloseHides>()
                    .is_some_and(|flag| flag.0.load(Ordering::Acquire));
                if hide {
                    api.prevent_close();
                    let _ = window.hide();
                }
            }
        })
        .invoke_handler(tauri::generate_handler![
            profile_list,
            profile_save,
            profile_forget,
            browse,
            transfer,
            diag,
            server_info,
            auth_login,
            auth_mfa,
            auth_logout,
            auth_oidc_password,
            auth_oidc_exchange,
            auth_oidc_browser,
            auth_api_token,
            auth_access_key,
            settings_load,
            settings_save
        ])
        .run(tauri::generate_context!())
        .expect("error while running DataSafeS3 Desktop");
}

#[cfg(test)]
mod tests {
    use super::{oidc_login_url, persisted_login_status};

    #[test]
    fn oidc_browser_url_uses_the_server_login_route() {
        assert_eq!(
            oidc_login_url("https://storage.example/").unwrap(),
            "https://storage.example/api/v1/auth/oidc/login"
        );
        assert_eq!(
            oidc_login_url("http://127.0.0.1:9000").unwrap(),
            "http://127.0.0.1:9000/api/v1/auth/oidc/login"
        );
        assert!(oidc_login_url("datasafe://oidc").is_err());
    }

    #[test]
    fn rejected_sign_in_is_not_persisted() {
        assert!(!persisted_login_status("rejected"));
        assert!(!persisted_login_status("blocked"));
        assert!(!persisted_login_status("error"));
        assert!(!persisted_login_status(""));
        assert!(persisted_login_status("signed_in"));
        assert!(persisted_login_status("mfa_required"));
    }
}
