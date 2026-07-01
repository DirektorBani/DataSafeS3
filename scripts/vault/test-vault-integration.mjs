/**
 * Vault profile integration smoke — injection path (Agent → env → healthy server).
 *
 * Skips when Vault is not reachable unless VAULT_PROFILE=1 (then fails).
 *
 * Env:
 *   VAULT_PROFILE=1          — require stack; exit 1 on failure (CI / smoke scripts)
 *   TEST_VAULT_ADDR          — Vault API (default http://127.0.0.1:8200)
 *   TEST_VAULT_TOKEN         — dev root token (default root)
 *   STORAGE_URL              — storage-server base (default http://127.0.0.1:9000)
 *   VAULT_KV_PATH            — KV v2 path (default secret/datasafe/bootstrap)
 *   VAULT_TEST_ADMIN_USER    — admin login user (default admin)
 *   VAULT_TEST_ADMIN_PASSWORD — expected injected admin password (from test-fixtures.env)
 */
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const fixturesPath = join(__dirname, "../../deploy/vault/test-fixtures.env");

function loadFixtures() {
  const out = {};
  const text = readFileSync(fixturesPath, "utf8");
  for (const line of text.split(/\r?\n/)) {
    const t = line.trim();
    if (!t || t.startsWith("#")) continue;
    const i = t.indexOf("=");
    if (i < 1) continue;
    out[t.slice(0, i)] = t.slice(i + 1);
  }
  return out;
}

const fixtures = loadFixtures();
const requireVault = process.env.VAULT_PROFILE === "1";
const vaultAddr = (process.env.TEST_VAULT_ADDR || "http://127.0.0.1:8200").replace(/\/$/, "");
const vaultToken = process.env.TEST_VAULT_TOKEN || "root";
const storageUrl = (process.env.STORAGE_URL || "http://127.0.0.1:9000").replace(/\/$/, "");
const kvPath = process.env.VAULT_KV_PATH || fixtures.VAULT_KV_PATH || "secret/datasafe/bootstrap";
const adminUser = process.env.VAULT_TEST_ADMIN_USER || fixtures.VAULT_TEST_ADMIN_USER || "admin";
const adminPassword =
  process.env.VAULT_TEST_ADMIN_PASSWORD || fixtures.VAULT_TEST_ADMIN_PASSWORD || "VaultIntegAdmin9!";

const results = [];

function record(name, ok, detail) {
  results.push({ name, ok, detail });
  const tag = ok ? "PASS" : "FAIL";
  console.log(`[${tag}] ${name}: ${detail}`);
}

async function vaultReachable() {
  try {
    const res = await fetch(`${vaultAddr}/v1/sys/health`);
    const body = await res.json();
    const unsealed = body.sealed === false || body.initialized === true;
    return { ok: res.ok || res.status === 200 || res.status === 429, body, unsealed };
  } catch (e) {
    return { ok: false, error: e };
  }
}

async function vaultKvHasSecrets() {
  const apiPath = kvPath.startsWith("secret/data/") ? kvPath : `secret/data/${kvPath.replace(/^secret\//, "")}`;
  const res = await fetch(`${vaultAddr}/v1/${apiPath}`, {
    headers: { "X-Vault-Token": vaultToken },
  });
  if (!res.ok) {
    return { ok: false, detail: `HTTP ${res.status}` };
  }
  const json = await res.json();
  const data = json?.data?.data || {};
  const keys = ["jwt_secret", "s3_secret_key", "admin_password"];
  const missing = keys.filter((k) => !data[k]);
  if (missing.length) {
    return { ok: false, detail: `missing keys: ${missing.join(", ")}` };
  }
  return { ok: true, detail: `keys=${keys.join(",")}` };
}

async function storageHealthz() {
  const res = await fetch(`${storageUrl}/healthz`);
  const text = await res.text();
  return { ok: res.ok, detail: `HTTP ${res.status} ${text.trim()}` };
}

async function securityStatusNoWeakDefaults() {
  const loginRes = await fetch(`${storageUrl}/api/v1/admin/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username: adminUser, password: adminPassword }),
  });
  if (!loginRes.ok) {
    const body = await loginRes.text();
    return { ok: false, detail: `login HTTP ${loginRes.status}: ${body}` };
  }
  const { token } = await loginRes.json();
  if (!token) {
    return { ok: false, detail: "login returned no token" };
  }

  const res = await fetch(`${storageUrl}/api/v1/settings/security-status`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!res.ok) {
    return { ok: false, detail: `security-status HTTP ${res.status}` };
  }
  const json = await res.json();
  const weak = json.weak_secrets || [];
  if (weak.length > 0) {
    return { ok: false, detail: `weak_secrets=${JSON.stringify(weak)}` };
  }
  return { ok: true, detail: "weak_secrets=[]" };
}

async function main() {
  const reach = await vaultReachable();
  if (!reach.ok) {
    const msg = reach.error ? String(reach.error) : `health HTTP not OK`;
    if (requireVault) {
      record("vault reachable", false, msg);
      process.exit(1);
    }
    console.log(`SKIP: Vault not reachable at ${vaultAddr} (${msg})`);
    console.log("Set VAULT_PROFILE=1 after starting compose --profile vault (see deploy/vault/README.md)");
    process.exit(0);
  }

  const mode = reach.body?.sealed === false ? "unsealed" : reach.body?.initialized ? "dev/initialized" : "unknown";
  record("vault reachable / unsealed", reach.unsealed, `mode=${mode}`);

  const kv = await vaultKvHasSecrets();
  record("vault KV secrets present", kv.ok, kv.detail);

  const hz = await storageHealthz();
  record("storage-server /healthz", hz.ok, hz.detail);

  let sec = { ok: false, detail: "skipped (healthz failed)" };
  if (hz.ok) {
    sec = await securityStatusNoWeakDefaults();
    record("security-status no weak defaults", sec.ok, sec.detail);
  } else {
    record("security-status no weak defaults", false, sec.detail);
  }

  const failed = results.filter((r) => !r.ok);
  if (failed.length) {
    process.exit(requireVault ? 1 : 0);
  }
  console.log("Vault integration smoke: all checks passed");
}

main().catch((e) => {
  console.error(e);
  process.exit(requireVault ? 1 : 0);
});
