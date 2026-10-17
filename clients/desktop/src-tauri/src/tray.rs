use std::thread;
use std::time::Duration;

use serde_json::Value;
use tauri::menu::{Menu, MenuItem};
use tauri::tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent};
use tauri::{App, Manager};

use crate::settings;
use crate::Desktop;

pub fn install(app: &App) -> bool {
    match build(app) {
        Ok(()) => true,
        Err(err) => {
            crate::logging::error(&format!("tray {err}"));
            false
        }
    }
}

fn build(app: &App) -> Result<(), String> {
    let locale = locale_of();
    let status = MenuItem::with_id(app, "status", status_line(&locale, 0, 0, 0, 0), false, None::<&str>).map_err(|err| err.to_string())?;
    let show = MenuItem::with_id(app, "show", label(&locale, "show"), true, None::<&str>).map_err(|err| err.to_string())?;
    let pause = MenuItem::with_id(app, "pause", label(&locale, "pause"), true, None::<&str>).map_err(|err| err.to_string())?;
    let quit = MenuItem::with_id(app, "quit", label(&locale, "quit"), true, None::<&str>).map_err(|err| err.to_string())?;
    let menu = Menu::with_items(app, &[&status, &show, &pause, &quit]).map_err(|err| err.to_string())?;
    let icon = app.default_window_icon().cloned().ok_or("window icon is missing")?;
    let tray = TrayIconBuilder::with_id("main")
        .menu(&menu)
        .icon(icon)
        .show_menu_on_left_click(true)
        .on_menu_event(|app, event| match event.id.as_ref() {
            "show" => show_main(app),
            "pause" => {
                let handle = app.clone();
                thread::spawn(move || {
                    if let Some(state) = handle.try_state::<Desktop>() {
                        if let Ok(mut agent) = state.agent.lock() {
                            let _ = agent.call("transfer.pause_all", serde_json::json!({}));
                        }
                    }
                });
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
                show_main(tray.app_handle());
            }
        })
        .build(app)
        .map_err(|err| err.to_string())?;
    let status_item = status.clone();
    let show_item = show.clone();
    let pause_item = pause.clone();
    let quit_item = quit.clone();
    let handle = app.handle().clone();
    thread::spawn(move || loop {
        thread::sleep(Duration::from_secs(2));
        if handle.try_state::<Desktop>().is_none() {
            break;
        }
        let locale = locale_of();
        let (profiles, signed, active, paused) = counts(&handle);
        if status_item.set_text(status_line(&locale, profiles, signed, active, paused)).is_err() {
            break;
        }
        let _ = show_item.set_text(label(&locale, "show"));
        let _ = pause_item.set_text(label(&locale, "pause"));
        let _ = quit_item.set_text(label(&locale, "quit"));
    });
    // The icon is removed when this value is dropped. It has to outlive setup.
    std::mem::forget(tray);
    Ok(())
}

fn show_main(app: &tauri::AppHandle) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.show();
        let _ = window.unminimize();
        let _ = window.set_focus();
    }
}

fn counts(app: &tauri::AppHandle) -> (u64, u64, u64, u64) {
    let Some(state) = app.try_state::<Desktop>() else {
        return (0, 0, 0, 0);
    };
    let Ok(mut agent) = state.agent.lock() else {
        return (0, 0, 0, 0);
    };
    let profiles = agent.call("profile.list", serde_json::json!({})).unwrap_or(serde_json::json!({}));
    let transfers = agent.call("transfer.list", serde_json::json!({})).unwrap_or(serde_json::json!({}));
    drop(agent);
    let mut total = 0u64;
    let mut signed = 0u64;
    if let Some(items) = profiles.get("profiles").and_then(Value::as_array) {
        for item in items {
            total += 1;
            if item.get("session").and_then(Value::as_str) == Some("signed_in") {
                signed += 1;
            }
        }
    }
    let mut active = 0u64;
    let mut paused = 0u64;
    if let Some(items) = transfers.get("transfers").and_then(Value::as_array) {
        for item in items {
            match item.get("state").and_then(Value::as_str).unwrap_or("") {
                "queued" | "running" | "waiting_network" => active += 1,
                "interrupted" | "conflict" => paused += 1,
                _ => {}
            }
        }
    }
    (total, signed, active, paused)
}

fn locale_of() -> String {
    settings::load().get("locale").and_then(Value::as_str).unwrap_or("en").to_string()
}

fn label(locale: &str, id: &str) -> String {
    let ru = locale == "ru";
    match id {
        "show" if ru => "Показать".to_string(),
        "show" => "Show".to_string(),
        "pause" if ru => "Пауза всех".to_string(),
        "pause" => "Pause all".to_string(),
        "quit" if ru => "Выход".to_string(),
        _ => "Quit".to_string(),
    }
}

pub(crate) fn status_line(locale: &str, profiles: u64, signed: u64, active: u64, paused: u64) -> String {
    if locale == "ru" {
        format!("{profiles} профилей · {signed} в сети · {active} идут · {paused} на паузе")
    } else {
        format!("{profiles} profiles · {signed} signed in · {active} running · {paused} paused")
    }
}

#[cfg(test)]
mod tests {
    use super::status_line;

    #[test]
    fn status_line_follows_the_locale() {
        assert_eq!(status_line("en", 2, 1, 1, 0), "2 profiles · 1 signed in · 1 running · 0 paused");
        assert_eq!(status_line("ru", 2, 1, 0, 1), "2 профилей · 1 в сети · 0 идут · 1 на паузе");
    }
}
