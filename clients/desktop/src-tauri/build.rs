fn main() {
    tauri_build::try_build(
        tauri_build::Attributes::new().app_manifest(
            tauri_build::AppManifest::new().commands(&[
                "profile_list",
                "profile_save",
                "profile_forget",
                "browse",
                "transfer",
                "server_info",
                "auth_login",
                "auth_mfa",
                "auth_logout",
                "auth_oidc_password",
                "auth_oidc_exchange",
                "auth_api_token",
                "auth_access_key",
            ]),
        ),
    )
    .expect("failed to run tauri-build");
    #[cfg(target_os = "windows")]
    embed_resource::compile("windows/longpath.rc", embed_resource::NONE);
}
