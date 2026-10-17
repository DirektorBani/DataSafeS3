import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@datasafe/ui/ui/button";
import { Input } from "@datasafe/ui/ui/input";
import { Label } from "@datasafe/ui/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@datasafe/ui/ui/card";
import { cn } from "@datasafe/ui/cn";
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
  const [caFile, setCaFile] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    try {
      await saveProfile({
        name,
        server_url: server,
        auth_method: method,
        username,
        tls: { insecure_dev: insecure, ca_file: caFile.trim() || undefined },
      });
      setName("");
      setUsername("");
      props.onCreated();
    } catch (err) {
      props.onError(err instanceof Error ? err.message : String(err));
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("profiles")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
      {props.profiles.length === 0 ? <p className="text-sm text-muted-foreground">{t("empty")}</p> : null}
      <ul className="space-y-2">
        {props.profiles.map((profile) => (
          <li key={profile.id}>
            <button
              type="button"
              className={cn("w-full rounded-md border px-3 py-2 text-left transition-colors hover:bg-accent", props.selected === profile.id ? "border-primary bg-primary/5" : "border-border")}
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
        <div className="space-y-1">
          <Label htmlFor="profile-name">{t("name")}</Label>
          <Input id="profile-name" value={name} onChange={(event) => setName(event.target.value)} required autoComplete="off" />
        </div>
        <div className="space-y-1">
          <Label htmlFor="profile-server">{t("server")}</Label>
          <Input id="profile-server" value={server} onChange={(event) => setServer(event.target.value)} required autoComplete="off" />
        </div>
        <div className="space-y-1">
          <Label htmlFor="profile-method">{t("method")}</Label>
          <select id="profile-method" className="flex h-10 w-full rounded-md border border-input bg-background px-3 text-sm" value={method} onChange={(event) => setMethod(event.target.value as AuthMethod)}>
            {METHODS.map((item) => (
              <option key={item} value={item}>
                {t(`methods.${item}`)}
              </option>
            ))}
          </select>
        </div>
        {method !== "api_token" ? (
          <div className="space-y-1">
            <Label htmlFor="profile-user">{method === "access_key" ? t("accessKey") : t("username")}</Label>
            <Input id="profile-user" value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="off" />
          </div>
        ) : null}
        <div className="space-y-1">
          <Label htmlFor="profile-ca">{t("caFile")}</Label>
          <Input id="profile-ca" value={caFile} onChange={(event) => setCaFile(event.target.value)} autoComplete="off" />
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" className="h-4 w-4" checked={insecure} onChange={(event) => setInsecure(event.target.checked)} />
          {t("insecure")}
        </label>
        {insecure ? <p className="text-sm text-destructive">{t("insecureWarning")}</p> : null}
        <Button type="submit">{t("save")}</Button>
      </form>
      </CardContent>
    </Card>
  );
}
