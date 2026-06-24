const state = {
  server: localStorage.getItem("ds_server") || "",
  token: localStorage.getItem("ds_token") || "",
  bucket: null,
  buckets: [],
  objects: [],
};

const app = document.getElementById("app");

function api(path, opts = {}) {
  return fetch(state.server.replace(/\/+$/, "") + path, {
    ...opts,
    headers: {
      ...(opts.body ? { "Content-Type": "application/json" } : {}),
      ...(state.token ? { Authorization: "Bearer " + state.token } : {}),
      ...opts.headers,
    },
  });
}

function renderLogin() {
  app.innerHTML = `
    <header><h1>DataSafeS3</h1></header>
    <form id="loginForm" class="card">
      <label>Server <input name="server" value="${state.server || "http://localhost:8080"}" required /></label>
      <label>User <input name="user" autocomplete="username" required /></label>
      <label>Password <input name="pass" type="password" autocomplete="current-password" required /></label>
      <button type="submit">Sign in</button>
      <p id="err" class="err"></p>
    </form>`;
  document.getElementById("loginForm").onsubmit = async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    state.server = fd.get("server").trim();
    const res = await api("/api/v1/admin/login", {
      method: "POST",
      body: JSON.stringify({ username: fd.get("user"), password: fd.get("pass") }),
    });
    const body = await res.json();
    if (!res.ok || body.mfa_required) {
      document.getElementById("err").textContent = body.error || "Login failed";
      return;
    }
    state.token = body.token;
    localStorage.setItem("ds_server", state.server);
    localStorage.setItem("ds_token", state.token);
    await loadBuckets();
    renderFiles();
  };
}

async function loadBuckets() {
  const res = await api("/api/v1/buckets?filter=all");
  const body = await res.json();
  state.buckets = body.buckets || [];
  state.bucket = state.buckets[0]?.name || null;
}

async function loadObjects() {
  if (!state.bucket) return;
  const res = await api(`/api/v1/buckets/${encodeURIComponent(state.bucket)}/objects`);
  const body = await res.json();
  state.objects = (body.objects || []).filter((o) => !o.key.endsWith("/") || o.size > 0);
}

function renderFiles() {
  app.innerHTML = `
    <header>
      <button id="back" type="button" aria-label="Buckets">☰</button>
      <h1>${state.bucket || "Files"}</h1>
      <label class="upload-btn">↑<input id="fileInput" type="file" hidden /></label>
    </header>
    <ul class="list" id="objList"></ul>`;
  const list = document.getElementById("objList");
  state.objects.forEach((o) => {
    const li = document.createElement("li");
    li.textContent = o.key;
    li.onclick = () => download(o.key);
    list.appendChild(li);
  });
  document.getElementById("back").onclick = renderBucketPicker;
  document.getElementById("fileInput").onchange = async (e) => {
    const file = e.target.files?.[0];
    if (!file || !state.bucket) return;
    const key = file.name;
    const res = await api(`/api/v1/buckets/${encodeURIComponent(state.bucket)}/objects/${encodeURIComponent(key)}`, {
      method: "PUT",
      headers: { "Content-Type": file.type || "application/octet-stream" },
      body: file,
    });
    if (!res.ok) alert("Upload failed");
    else await loadObjects(), renderFiles();
  };
  loadObjects().then(() => {
    list.innerHTML = "";
    state.objects.forEach((o) => {
      const li = document.createElement("li");
      li.textContent = `${o.key} (${o.size}b)`;
      li.onclick = () => download(o.key);
      list.appendChild(li);
    });
  });
}

function renderBucketPicker() {
  app.innerHTML = `<header><h1>Buckets</h1></header><ul class="list" id="bucketList"></ul>`;
  const ul = document.getElementById("bucketList");
  state.buckets.forEach((b) => {
    const li = document.createElement("li");
    li.textContent = b.name;
    li.onclick = () => { state.bucket = b.name; renderFiles(); };
    ul.appendChild(li);
  });
}

async function download(key) {
  const res = await api(`/api/v1/buckets/${encodeURIComponent(state.bucket)}/objects/${key.split("/").map(encodeURIComponent).join("/")}`);
  if (!res.ok) return alert("Download failed");
  const blob = await res.blob();
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = key.split("/").pop();
  a.click();
}

if (state.token) {
  loadBuckets().then(renderBucketPicker).catch(renderLogin);
} else {
  renderLogin();
}
