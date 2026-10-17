//! Seeds Windows Credential Manager through the same keyring call the desktop
//! uses: Entry::new("datasafe-desktop", "{profile_id}/{kind}"). The
//! windows-native store persists that as "{profile_id}/{kind}.datasafe-desktop".
//! desktop-windows-smoke.ps1 runs this, then datasafe-agent --purge-secrets.

fn main() {
    let mut args = std::env::args().skip(1);
    let cmd = args.next().unwrap_or_default();
    let id = args.next().unwrap_or_default();
    if id.is_empty() || id.chars().any(|c| matches!(c, ':' | '=' | ' ' | '/' | '\\')) {
        eprintln!("profile id");
        std::process::exit(2);
    }
    let code = match cmd.as_str() {
        "seed" => seed(&id),
        "expect-present" => expect_present(&id),
        "expect-purged" => expect_purged(&id),
        "cleanup" => cleanup(&id),
        _ => {
            eprintln!("command");
            2
        }
    };
    std::process::exit(code);
}

fn entry(profile_id: &str, kind: &str) -> Result<keyring::Entry, String> {
    keyring::Entry::new("datasafe-desktop", &format!("{profile_id}/{kind}")).map_err(|err| err.to_string())
}

fn nested(id: &str) -> String {
    format!("{id}/x")
}

fn read(profile_id: &str, kind: &str) -> Result<Option<String>, String> {
    match entry(profile_id, kind)?.get_password() {
        Ok(value) => Ok(Some(value)),
        Err(keyring::Error::NoEntry) => Ok(None),
        Err(err) => Err(err.to_string()),
    }
}

fn write(profile_id: &str, kind: &str, value: &str) -> Result<(), String> {
    entry(profile_id, kind)?.set_password(value).map_err(|err| err.to_string())
}

fn remove(profile_id: &str, kind: &str) -> Result<(), String> {
    match entry(profile_id, kind)?.delete_credential() {
        Ok(()) => Ok(()),
        Err(keyring::Error::NoEntry) => Ok(()),
        Err(err) => Err(err.to_string()),
    }
}

fn seed(id: &str) -> i32 {
    if let Err(err) = (|| {
        write(id, "password", "ci-password")?;
        write(id, "token", "ci-token")?;
        write(&nested(id), "password", "ci-keep")?;
        Ok::<(), String>(())
    })() {
        eprintln!("seed: {err}");
        return 1;
    }
    println!("seeded");
    0
}

fn expect_present(id: &str) -> i32 {
    match present(id) {
        Ok(()) => {
            println!("present");
            0
        }
        Err(err) => {
            eprintln!("{err}");
            1
        }
    }
}

fn present(id: &str) -> Result<(), String> {
    expect_value(id, "password", Some("ci-password"))?;
    expect_value(id, "token", Some("ci-token"))?;
    expect_value(&nested(id), "password", Some("ci-keep"))?;
    Ok(())
}

fn expect_purged(id: &str) -> i32 {
    match purged(id) {
        Ok(()) => {
            println!("purged-ok");
            0
        }
        Err(err) => {
            eprintln!("{err}");
            1
        }
    }
}

fn purged(id: &str) -> Result<(), String> {
    expect_value(id, "password", None)?;
    expect_value(id, "token", None)?;
    expect_value(&nested(id), "password", Some("ci-keep"))?;
    Ok(())
}

fn expect_value(profile_id: &str, kind: &str, want: Option<&str>) -> Result<(), String> {
    let got = read(profile_id, kind)?;
    match (want, got.as_deref()) {
        (None, None) => Ok(()),
        (Some(value), Some(saved)) if saved == value => Ok(()),
        (None, Some(_)) => Err(format!("{profile_id}/{kind} is still in the credential store")),
        (Some(_), None) => Err(format!("{profile_id}/{kind} is missing")),
        (Some(_), Some(_)) => Err(format!("{profile_id}/{kind} value changed")),
    }
}

fn cleanup(id: &str) -> i32 {
    let nested_id = nested(id);
    let mut failed = false;
    for (profile, kind) in [(id, "password"), (id, "token"), (nested_id.as_str(), "password")] {
        if let Err(err) = remove(profile, kind) {
            eprintln!("cleanup {profile}/{kind}: {err}");
            failed = true;
        }
    }
    if failed {
        1
    } else {
        println!("cleaned");
        0
    }
}
