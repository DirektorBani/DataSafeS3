fn main() {
    // One RT_MANIFEST. Tauri already embeds resource 1; a second file
    // (embed-resource of longpath.rc) makes cvtres fail with CVT1100.
    let windows = tauri_build::WindowsAttributes::new()
        .app_manifest(include_str!("windows/longpath.manifest"));
    tauri_build::try_build(
        tauri_build::Attributes::new()
            .windows_attributes(windows)
            .app_manifest(tauri_build::AppManifest::new().commands(&[
                "profile_list",
                "profile_save",
                "profile_forget",
                "browse",
                "transfer",
                "diag",
                "server_info",
                "auth_login",
                "auth_mfa",
                "auth_logout",
                "auth_oidc_password",
                "auth_oidc_exchange",
                "auth_oidc_browser",
                "auth_api_token",
                "auth_access_key",
                "settings_load",
                "settings_save",
            ])),
    )
    .expect("failed to run tauri-build");
}
