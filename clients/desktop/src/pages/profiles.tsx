import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@datasafe/ui/ui/button";
import { saveProfile, type AuthMethod, type Profile } from "../desktop";

const METHODS: AuthMethod[] = ["password", "oidc", "api_token", "access_key"];

export function ProfileList(props: {
  profiles: Profile[];
  selected: string;
  onSelect: (id: string) => void;
  onCreated: () => void;
  onError: (message: string) => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [server, setServer] = useState("https://");
  const [method, setMethod] = useState<AuthMethod>("password");
  const [username, setUsername] = useState("");
  const [insecure, setInsecure] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    try {
      await saveProfile({
        name,
        server_url: server,
        auth_method: method,
        username,
        tls: { insecure_dev: insecure },
      });
      setName("");
      setUsername("");
      props.onCreated();
    } catch (err) {
      props.onError(err instanceof Error ? err.message : String(err));
    }
  }

  return (
    <section className="space-y-4">
      <h2 className="text-sm font-medium">{t("profiles")}</h2>
      {props.profiles.length === 0 ? <p className="text-sm text-muted-foreground">{t("empty")}</p> : null}
      <ul className="space-y-2">
        {props.profiles.map((profile) => (
          <li key={profile.id}>
            <button
              type="button"
              className={`w-full rounded-md border px-3 py-2 text-left ${props.selected === profile.id ? "border-primary" : "border-border"}`}
              onClick={() => props.onSelect(profile.id)}
            >
              <span className="block font-medium">{profile.name}</span>
              <span className="block text-sm text-muted-foreground">
                {profile.server_url} · {t(`methods.${profile.auth_method}`)} · {profile.session || "signed_out"}
              </span>
            </button>
          </li>
        ))}
      </ul>
      <form className="space-y-3" onSubmit={submit}>
        <h3 className="text-sm font-medium">{t("addProfile")}</h3>
        <label className="block text-sm">
          {t("name")}
          <input className="mt-1 w-full rounded-md border bg-background px-2 py-1" value={name} onChange={(event) => setName(event.target.value)} required />
        </label>
        <label className="block text-sm">
          {t("server")}
          <input className="mt-1 w-full rounded-md border bg-background px-2 py-1" value={server} onChange={(event) => setServer(event.target.value)} required />
        </label>
        <label className="block text-sm">
          {t("method")}
          <select className="mt-1 w-full rounded-md border bg-background px-2 py-1" value={method} onChange={(event) => setMethod(event.target.value as AuthMethod)}>
            {METHODS.map((item) => (
              <option key={item} value={item}>
                {t(`methods.${item}`)}
              </option>
            ))}
          </select>
        </label>
        {method !== "api_token" ? (
          <label className="block text-sm">
            {method === "access_key" ? t("accessKey") : t("username")}
            <input className="mt-1 w-full rounded-md border bg-background px-2 py-1" value={username} onChange={(event) => setUsername(event.target.value)} />
          </label>
        ) : null}
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={insecure} onChange={(event) => setInsecure(event.target.checked)} />
          {t("insecure")}
        </label>
        <Button type="submit">{t("save")}</Button>
      </form>
    </section>
  );
}
