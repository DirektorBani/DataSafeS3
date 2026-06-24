use serde::Serialize;
use std::process::Child;
use std::sync::Mutex;
use tauri::{
    menu::{Menu, MenuItem},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    AppHandle, Emitter, Manager, State,
};
use tauri_plugin_shell::{process::CommandEvent, ShellExt};

struct WatchState(Mutex<Option<Child>>);

#[derive(Clone, Serialize)]
struct SyncLine {
    line: String,
}

fn sidecar(app: &AppHandle) -> Result<tauri_plugin_shell::process::Command, String> {
    app.shell()
        .sidecar("datasafe-sync")
        .map_err(|e| format!("sidecar missing — build datasafe-sync into src-tauri/binaries/: {e}"))
}

#[tauri::command]
async fn get_status(app: AppHandle) -> Result<String, String> {
    let (mut rx, _child) = sidecar(&app)?
        .args(["status", "--json"])
        .spawn()
        .map_err(|e| e.to_string())?;
    let mut stdout = String::new();
    while let Some(event) = rx.recv().await {
        if let CommandEvent::Stdout(line) = event {
            stdout.push_str(&String::from_utf8_lossy(&line));
        }
    }
    Ok(stdout)
}

#[tauri::command]
async fn login(
    app: AppHandle,
    server: String,
    username: String,
    password: String,
) -> Result<String, String> {
    let (mut rx, _child) = sidecar(&app)?
        .args([
            "login",
            "--server",
            &server,
            "--user",
            &username,
            "--password",
            &password,
        ])
        .spawn()
        .map_err(|e| e.to_string())?;
    let mut stdout = String::new();
    let mut stderr = String::new();
    while let Some(event) = rx.recv().await {
        match event {
            CommandEvent::Stdout(line) => stdout.push_str(&String::from_utf8_lossy(&line)),
            CommandEvent::Stderr(line) => stderr.push_str(&String::from_utf8_lossy(&line)),
            CommandEvent::Terminated(payload) => {
                if payload.code != Some(0) {
                    return Err(format!("{stderr}{stdout}"));
                }
            }
            _ => {}
        }
    }
    Ok(stdout.trim().to_string())
}

#[tauri::command]
async fn list_buckets(app: AppHandle) -> Result<String, String> {
    let (mut rx, _child) = sidecar(&app)?
        .args(["buckets", "--json"])
        .spawn()
        .map_err(|e| e.to_string())?;
    let mut stdout = String::new();
    while let Some(event) = rx.recv().await {
        if let CommandEvent::Stdout(line) = event {
            stdout.push_str(&String::from_utf8_lossy(&line));
        }
    }
    Ok(stdout)
}

#[tauri::command]
async fn run_sync(
    app: AppHandle,
    folder: String,
    bucket: String,
    prefix: String,
    delete_sync: bool,
    conflict_policy: String,
) -> Result<String, String> {
    let mut args = vec![
        "sync".to_string(),
        "--folder".to_string(),
        folder,
        "--bucket".to_string(),
        bucket,
        "--prefix".to_string(),
        prefix,
        "--json-lines".to_string(),
        "--conflict-policy".to_string(),
        conflict_policy,
    ];
    if delete_sync {
        args.push("--delete".to_string());
    }
    let (mut rx, _child) = sidecar(&app)?
        .args(args)
        .spawn()
        .map_err(|e| e.to_string())?;
    let mut stdout = String::new();
    let mut stderr = String::new();
    let window = app.get_webview_window("main");
    while let Some(event) = rx.recv().await {
        match event {
            CommandEvent::Stdout(line) => {
                let text = String::from_utf8_lossy(&line).to_string();
                stdout.push_str(&text);
                if let Some(w) = &window {
                    let _ = w.emit("sync-line", SyncLine { line: text });
                }
            }
            CommandEvent::Stderr(line) => stderr.push_str(&String::from_utf8_lossy(&line)),
            CommandEvent::Terminated(payload) => {
                if payload.code != Some(0) {
                    return Err(format!("{stderr}{stdout}"));
                }
            }
            _ => {}
        }
    }
    Ok(stdout)
}

#[tauri::command]
async fn start_watch(
    app: AppHandle,
    state: State<'_, WatchState>,
    folder: String,
    bucket: String,
    prefix: String,
    interval_secs: u64,
    use_fsnotify: bool,
    delete_sync: bool,
    conflict_policy: String,
) -> Result<(), String> {
    let mut guard = state.0.lock().map_err(|e| e.to_string())?;
    if guard.is_some() {
        return Err("watch already running".into());
    }
    let mut args = vec![
        "watch".to_string(),
        "--folder".to_string(),
        folder,
        "--bucket".to_string(),
        bucket,
        "--prefix".to_string(),
        prefix,
        "--interval".to_string(),
        format!("{}s", interval_secs.max(1)),
        "--conflict-policy".to_string(),
        conflict_policy,
    ];
    if use_fsnotify {
        args.push("--fsnotify".to_string());
    }
    if delete_sync {
        args.push("--delete".to_string());
    }
    let (_rx, child) = sidecar(&app)?.args(args).spawn().map_err(|e| e.to_string())?;
    *guard = Some(child);
    Ok(())
}

#[tauri::command]
async fn stop_watch(state: State<'_, WatchState>) -> Result<(), String> {
    let mut guard = state.0.lock().map_err(|e| e.to_string())?;
    if let Some(mut child) = guard.take() {
        let _ = child.kill();
    }
    Ok(())
}

#[tauri::command]
async fn list_conflicts(app: AppHandle, folder: String) -> Result<String, String> {
    let (mut rx, _child) = sidecar(&app)?
        .args(["conflicts", "--folder", &folder])
        .spawn()
        .map_err(|e| e.to_string())?;
    let mut stdout = String::new();
    while let Some(event) = rx.recv().await {
        if let CommandEvent::Stdout(line) = event {
            stdout.push_str(&String::from_utf8_lossy(&line));
        }
    }
    Ok(stdout)
}

fn setup_tray(app: &AppHandle) -> Result<(), Box<dyn std::error::Error>> {
    let show = MenuItem::with_id(app, "show", "Show DataSafeS3 Sync", true, None::<&str>)?;
    let quit = MenuItem::with_id(app, "quit", "Quit", true, None::<&str>)?;
    let menu = Menu::with_items(app, &[&show, &quit])?;
    let _tray = TrayIconBuilder::new()
        .menu(&menu)
        .tooltip("DataSafeS3 Sync")
        .on_menu_event(|app, event| match event.id.as_ref() {
            "show" => {
                if let Some(w) = app.get_webview_window("main") {
                    let _ = w.show();
                    let _ = w.set_focus();
                }
            }
            "quit" => app.exit(0),
            _ => {}
        })
        .on_tray_icon_event(|tray, event| {
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = event
            {
                let app = tray.app_handle();
                if let Some(w) = app.get_webview_window("main") {
                    let _ = w.show();
                    let _ = w.set_focus();
                }
            }
        })
        .build(app)?;
    Ok(())
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_dialog::init())
        .manage(WatchState(Mutex::new(None)))
        .invoke_handler(tauri::generate_handler![
            get_status,
            login,
            list_buckets,
            run_sync,
            start_watch,
            stop_watch,
            list_conflicts,
        ])
        .setup(|app| {
            setup_tray(app.handle())?;
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
