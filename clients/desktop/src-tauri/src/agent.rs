use serde_json::{json, Value};
use std::io::{BufRead, BufReader, Write};
use std::path::{Path, PathBuf};
use std::process::{Child, ChildStdin, Command, Stdio};
use std::thread;

pub struct Agent {
    child: Option<Child>,
    stdin: Option<ChildStdin>,
    stdout: Option<BufReader<std::process::ChildStdout>>,
    next_id: u64,
    path: Option<PathBuf>,
    data_dir: PathBuf,
    start_error: Option<String>,
}

impl Agent {
    pub fn open(data_dir: PathBuf, resource_dir: Option<PathBuf>) -> Self {
        let path = find_agent(resource_dir).ok();
        let mut agent = Self {
            child: None,
            stdin: None,
            stdout: None,
            next_id: 0,
            path,
            data_dir,
            start_error: None,
        };
        if let Err(err) = agent.spawn() {
            agent.start_error = Some(err);
        }
        agent
    }

    pub fn call(&mut self, method: &str, params: Value) -> Result<Value, String> {
        if self.stdin.is_none() {
            self.spawn()?;
            self.start_error = None;
        }
        let id = {
            self.next_id += 1;
            self.next_id
        };
        let request = json!({
            "jsonrpc": "2.0",
            "id": id,
            "method": method,
            "params": params,
        });
        let line = serde_json::to_string(&request).map_err(|err| err.to_string())?;
        let wrote = match self.stdin.as_mut() {
            Some(stdin) => writeln!(stdin, "{line}").is_ok(),
            None => false,
        };
        if !wrote {
            self.spawn()?;
            let stdin = self.stdin.as_mut().ok_or("agent is not running")?;
            writeln!(stdin, "{line}").map_err(|err| err.to_string())?;
        }
        let stdout = self.stdout.as_mut().ok_or("agent is not running")?;
        let mut response = String::new();
        if stdout.read_line(&mut response).map_err(|err| err.to_string())? == 0 {
            return Err("agent closed the pipe".into());
        }
        let parsed: Value = serde_json::from_str(&response).map_err(|err| err.to_string())?;
        if let Some(message) = parsed.get("error").and_then(|err| err.get("message")).and_then(Value::as_str) {
            return Err(message.to_string());
        }
        parsed.get("result").cloned().ok_or_else(|| "agent returned no result".into())
    }

    fn spawn(&mut self) -> Result<(), String> {
        if let Some(err) = &self.start_error {
            if self.path.is_none() {
                return Err(err.clone());
            }
        }
        if let Some(mut child) = self.child.take() {
            let _ = child.kill();
            let _ = child.wait();
        }
        let path = self.path.clone().ok_or("datasafe-agent was not found next to the app")?;
        let mut command = Command::new(&path);
        command
            .arg("--data-dir")
            .arg(&self.data_dir)
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped());
        #[cfg(windows)]
        {
            use std::os::windows::process::CommandExt;
            // A console-subsystem agent would flash a window beside the GUI shell.
            command.creation_flags(0x08000000);
        }
        let mut child = command.spawn().map_err(|err| format!("could not start the agent: {err}"))?;
        if let Some(stderr) = child.stderr.take() {
            thread::spawn(move || {
                let _ = std::io::copy(&mut BufReader::new(stderr), &mut std::io::sink());
            });
        }
        self.stdin = child.stdin.take();
        self.stdout = child.stdout.take().map(BufReader::new);
        self.child = Some(child);
        Ok(())
    }
}

fn find_agent(resource_dir: Option<PathBuf>) -> Result<PathBuf, String> {
    let mut candidates = Vec::new();
    if let Ok(path) = std::env::var("DATASAFE_AGENT") {
        if !path.is_empty() {
            candidates.push(PathBuf::from(path));
        }
    }
    if let Some(dir) = resource_dir {
        push_near(&dir, &mut candidates);
        push_near(&dir.join("binaries"), &mut candidates);
    }
    let names = ["datasafe-agent", "datasafe-agent.exe"];
    let manifest = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("binaries");
    for name in names {
        candidates.push(manifest.join(name));
    }
    if let Ok(exe) = std::env::current_exe() {
        if let Some(dir) = exe.parent() {
            push_near(dir, &mut candidates);
            push_near(&dir.join("binaries"), &mut candidates);
        }
    }
    candidates
        .into_iter()
        .find(|path| path.is_file())
        .ok_or_else(|| "datasafe-agent was not found next to the app".into())
}

fn push_near(dir: &Path, candidates: &mut Vec<PathBuf>) {
    candidates.push(dir.join("datasafe-agent"));
    candidates.push(dir.join("datasafe-agent.exe"));
}
