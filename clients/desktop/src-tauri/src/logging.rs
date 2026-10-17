use std::fs::{self, File, OpenOptions};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::Mutex;
use std::time::{SystemTime, UNIX_EPOCH};

struct RotatingLog {
    path: PathBuf,
    file: File,
    size: u64,
    max_bytes: u64,
    max_files: u32,
    min_level: u8,
}

static LOG: Mutex<Option<RotatingLog>> = Mutex::new(None);

const KEYS: &[&str] = &[
    "authorization",
    "x-datasafe-mfa-token",
    "x-amz-signature",
    "x-amz-credential",
    "x-amz-security-token",
    "password",
    "secret",
    "token",
    "mfa_code",
    "mfa_token",
    "access_key_secret",
    "exchange_code",
];

pub fn init(dir: &Path) {
    let _ = fs::create_dir_all(dir);
    let path = dir.join("desktop.log");
    let mut max_bytes = 10 * 1024 * 1024;
    let mut max_files = 5u32;
    let mut min_level = 1u8;
    if let Ok(raw) = std::env::var("DATASAFE_DESKTOP_LOG_MAX_MB") {
        if let Ok(n) = raw.trim().parse::<u64>() {
            if n > 0 {
                max_bytes = n * 1024 * 1024;
            }
        }
    }
    if let Ok(raw) = std::env::var("DATASAFE_DESKTOP_LOG_MAX_FILES") {
        if let Ok(n) = raw.trim().parse::<u32>() {
            if n > 1 {
                max_files = n;
            }
        }
    }
    if let Ok(raw) = std::env::var("DATASAFE_DESKTOP_LOG_LEVEL") {
        min_level = match raw.trim().to_ascii_lowercase().as_str() {
            "debug" => 0,
            "warn" | "warning" => 2,
            "error" => 3,
            _ => 1,
        };
    }
    let file = match OpenOptions::new().create(true).append(true).open(&path) {
        Ok(file) => file,
        Err(_) => return,
    };
    let size = file.metadata().map(|meta| meta.len()).unwrap_or(0);
    if let Ok(mut slot) = LOG.lock() {
        *slot = Some(RotatingLog {
            path,
            file,
            size,
            max_bytes,
            max_files,
            min_level,
        });
    }
}

pub fn info(msg: &str) {
    write_line(1, "INFO", msg);
}

pub fn error(msg: &str) {
    write_line(3, "ERROR", msg);
}

fn write_line(level: u8, name: &str, msg: &str) {
    let Ok(mut slot) = LOG.lock() else {
        return;
    };
    let Some(log) = slot.as_mut() else {
        return;
    };
    if level < log.min_level {
        return;
    }
    let secs = SystemTime::now().duration_since(UNIX_EPOCH).map(|d| d.as_secs()).unwrap_or(0);
    let line = format!("{secs} {name} {}\n", mask(msg));
    if log.size > 0 && log.size + line.len() as u64 > log.max_bytes {
        rotate(log);
    }
    if log.file.write_all(line.as_bytes()).is_ok() {
        log.size += line.len() as u64;
    }
}

fn rotate(log: &mut RotatingLog) {
    let _ = log.file.flush();
    let oldest = numbered(&log.path, log.max_files - 1);
    let _ = fs::remove_file(oldest);
    let mut index = log.max_files - 1;
    while index > 1 {
        let _ = fs::rename(numbered(&log.path, index - 1), numbered(&log.path, index));
        index -= 1;
    }
    let _ = fs::rename(&log.path, numbered(&log.path, 1));
    if let Ok(file) = OpenOptions::new().create(true).append(true).open(&log.path) {
        log.file = file;
        log.size = 0;
    }
}

fn numbered(path: &Path, n: u32) -> PathBuf {
    PathBuf::from(format!("{}.{}", path.display(), n))
}

/// mask removes secrets before a line is written. Presigned query strings
/// are dropped whole.
pub fn mask(input: &str) -> String {
    let mut out = String::with_capacity(input.len());
    let mut i = 0;
    let bytes = input.as_bytes();
    while i < bytes.len() {
        if let Some(end) = http_end(input, i) {
            let url = &input[i..end];
            if let Some(q) = url.find('?') {
                let query = url[q..].to_ascii_lowercase();
                if query.contains("x-amz-") || query.contains("password") || query.contains("secret") || query.contains("token") {
                    out.push_str(&url[..q]);
                    out.push_str("?[redacted]");
                    i = end;
                    continue;
                }
            }
        }
        if let Some((kept, next)) = secret_value(input, i) {
            out.push_str(kept);
            out.push_str("[redacted]");
            i = next;
            continue;
        }
        let ch = input[i..].chars().next().unwrap_or('\u{fffd}');
        out.push(ch);
        i += ch.len_utf8();
    }
    out
}

fn http_end(input: &str, i: usize) -> Option<usize> {
    let rest = &input[i..];
    let lower = rest.as_bytes();
    let https = lower.len() >= 8 && lower[..8].eq_ignore_ascii_case(b"https://");
    let http = lower.len() >= 7 && lower[..7].eq_ignore_ascii_case(b"http://");
    if !https && !http {
        return None;
    }
    let end = rest.find([' ', '\t', '\n', '\r', '"', '\'', '<', '>']).map(|n| i + n).unwrap_or(input.len());
    Some(end)
}

fn secret_value(input: &str, i: usize) -> Option<(&str, usize)> {
    let rest = &input[i..];
    let lower = rest.to_ascii_lowercase();
    for key in KEYS {
        if !lower.starts_with(key) {
            continue;
        }
        let mut n = key.len();
        let tail = rest[n..].as_bytes();
        if tail.first() == Some(&b'"') {
            n += 1;
        }
        let after_key = rest[n..].trim_start();
        let skipped = rest[n..].len() - after_key.len();
        n += skipped;
        let sep = rest.as_bytes().get(n).copied()?;
        if sep != b':' && sep != b'=' {
            continue;
        }
        n += 1;
        let value = rest[n..].trim_start();
        n += rest[n..].len() - value.len();
        let value_len = if value.as_bytes().first() == Some(&b'"') {
            value[1..].find('"').map(|end| end + 2).unwrap_or(value.len())
        } else if key.eq_ignore_ascii_case("authorization") {
            let mut len = 0;
            let raw = value.as_bytes();
            if value.len() >= 7 && raw[..7].eq_ignore_ascii_case(b"bearer ") {
                len = 7;
                while len < raw.len() && !raw[len].is_ascii_whitespace() {
                    len += 1;
                }
            } else {
                while len < raw.len() && !raw[len].is_ascii_whitespace() && raw[len] != b',' && raw[len] != b'}' && raw[len] != b'&' {
                    len += 1;
                }
            }
            len
        } else {
            let raw = value.as_bytes();
            let mut len = 0;
            while len < raw.len() && !raw[len].is_ascii_whitespace() && raw[len] != b',' && raw[len] != b'}' && raw[len] != b'&' {
                len += 1;
            }
            len
        };
        if value_len == 0 {
            continue;
        }
        return Some((&rest[..n], i + n + value_len));
    }
    None
}

#[cfg(test)]
mod tests {
    use super::mask;

    #[test]
    fn masks_password_totp_and_presign() {
        let line = mask("password=s3cret-password X-DataSafe-MFA-Token: 918273 https://storage.example/b/k?X-Amz-Credential=AKIASECRET&X-Amz-Signature=deadbeefsignature");
        assert!(!line.contains("s3cret-password"), "{line}");
        assert!(!line.contains("918273"), "{line}");
        assert!(!line.contains("AKIASECRET"), "{line}");
        assert!(!line.contains("deadbeefsignature"), "{line}");
        assert!(line.contains("[redacted]"), "{line}");
    }
}
