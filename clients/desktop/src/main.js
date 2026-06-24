import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { open } from "@tauri-apps/plugin-dialog";

const $ = (id) => document.getElementById(id);

const log = (msg) => {
  const el = $("log");
  el.textContent += `${new Date().toLocaleTimeString()} ${msg}\n`;
  el.scrollTop = el.scrollHeight;
};

const setBadge = (text, kind = "muted") => {
  const el = $("statusBadge");
  el.textContent = text;
  el.className = `badge badge-${kind}`;
};

async function loadStatus() {
  try {
    const raw = await invoke("get_status");
    const st = JSON.parse(raw);
    $("server").value = st.profile_detail?.server_url || $("server").value;
    $("user").value = st.profile_detail?.username || "";
    $("folder").value = st.profile_detail?.folder || "";
    $("bucket").value = st.profile_detail?.bucket || "files";
    $("prefix").value = st.profile_detail?.prefix || "";
    if (st.logged_in) {
      setBadge(`Signed in · ${st.tracked_files} tracked files`, "ok");
      await loadBuckets();
    } else {
      setBadge("Not signed in", "warn");
    }
  } catch (e) {
    setBadge("CLI unavailable — use datasafe-sync on PATH for dev", "warn");
    log(String(e));
  }
}

async function loadBuckets() {
  try {
    const raw = await invoke("list_buckets");
    const data = JSON.parse(raw);
    const sel = $("bucket");
    const current = sel.value;
    sel.innerHTML = "";
    for (const b of data.buckets || []) {
      const opt = document.createElement("option");
      opt.value = b.name;
      let label = b.name;
      if (b.access?.ownership === "shared" && b.access.shared_by) {
        label += ` (shared by ${b.access.shared_by})`;
      } else if (b.access?.ownership) {
        label += ` (${b.access.ownership})`;
      }
      opt.textContent = label;
      sel.appendChild(opt);
    }
    if ([...sel.options].some((o) => o.value === current)) {
      sel.value = current;
    }
  } catch (e) {
    log(`buckets: ${e}`);
  }
}

async function refreshConflicts() {
  const folder = $("folder").value.trim();
  if (!folder) return;
  const ul = $("conflictList");
  ul.innerHTML = "";
  try {
    const raw = await invoke("list_conflicts", { folder });
    const lines = raw.trim().split("\n").filter(Boolean);
    if (lines.length === 1 && lines[0] === "no conflict backups") {
      ul.innerHTML = "<li class='muted'>No conflicts</li>";
      return;
    }
    for (const line of lines) {
      const li = document.createElement("li");
      li.textContent = line;
      ul.appendChild(li);
    }
  } catch (e) {
    log(`conflicts: ${e}`);
  }
}

function syncParams() {
  return {
    folder: $("folder").value.trim(),
    bucket: $("bucket").value.trim() || "files",
    prefix: $("prefix").value.trim(),
    deleteSync: $("deleteSync").checked,
    conflictPolicy: $("conflictPolicy").value,
  };
}

listen("sync-line", (ev) => {
  const line = ev.payload?.line?.trim();
  if (!line) return;
  try {
    const j = JSON.parse(line);
    if (j.type === "progress" && j.progress?.file) {
      log(`${j.progress.action}: ${j.progress.file}`);
    } else if (j.type === "done" && j.result) {
      const r = j.result;
      log(`done: ↑${r.uploaded} ↓${r.downloaded} conflicts=${(r.conflicts || []).length}`);
    }
  } catch {
    log(line);
  }
});

$("pickFolderBtn")?.addEventListener("click", async () => {
  const selected = await open({ directory: true, multiple: false });
  if (selected) {
    $("folder").value = selected;
  }
});

$("loginBtn")?.addEventListener("click", async () => {
  const server = $("server").value.trim();
  const username = $("user").value.trim();
  const password = $("password").value;
  if (!username || !password) {
    log("Enter username and password.");
    return;
  }
  try {
    const out = await invoke("login", { server, username, password });
    log(out || "Signed in.");
    setBadge("Signed in", "ok");
    await loadStatus();
  } catch (e) {
    log(String(e));
  }
});

$("syncBtn")?.addEventListener("click", async () => {
  const p = syncParams();
  if (!p.folder) {
    log("Choose a local folder first.");
    return;
  }
  log("Sync started…");
  try {
    await invoke("run_sync", p);
    log("Sync finished.");
    await loadStatus();
    await refreshConflicts();
  } catch (e) {
    log(String(e));
  }
});

$("watchStartBtn")?.addEventListener("click", async () => {
  const p = syncParams();
  if (!p.folder) {
    log("Choose a local folder first.");
    return;
  }
  try {
    await invoke("start_watch", {
      ...p,
      intervalSecs: Number($("interval").value) || 15,
      useFsnotify: $("useFsnotify").checked,
    });
    $("watchStartBtn").disabled = true;
    $("watchStopBtn").disabled = false;
    log("Background watch started.");
  } catch (e) {
    log(String(e));
  }
});

$("watchStopBtn")?.addEventListener("click", async () => {
  try {
    await invoke("stop_watch");
    $("watchStartBtn").disabled = false;
    $("watchStopBtn").disabled = true;
    log("Watch stopped.");
  } catch (e) {
    log(String(e));
  }
});

$("refreshConflictsBtn")?.addEventListener("click", refreshConflicts);

loadStatus();
refreshConflicts();
