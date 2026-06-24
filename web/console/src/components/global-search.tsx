import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { File, FolderOpen, Search, User, X } from "lucide-react";
import { api, type SearchResult } from "@/lib/api";
import { formatBytes } from "@/lib/utils";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

export function GlobalSearch() {
  const { t, i18n } = useTranslation("search");
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [debounced, setDebounced] = useState("");
  const [offset, setOffset] = useState(0);
  const navigate = useNavigate();
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const timer = setTimeout(() => {
      setDebounced(query.trim());
      setOffset(0);
    }, 250);
    return () => clearTimeout(timer);
  }, [query]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "k") {
        e.preventDefault();
        setOpen(true);
      }
      if (e.key === "Escape") setOpen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => {
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, []);

  const search = useQuery({
    queryKey: ["search", debounced, offset],
    queryFn: () => api.search(debounced, offset, 15),
    enabled: debounced.length >= 1,
  });

  const go = (r: SearchResult) => {
    setOpen(false);
    setQuery("");
    if (r.type === "bucket") navigate(`/buckets/${encodeURIComponent(r.bucket ?? r.name)}`);
    else if (r.type === "object" && r.bucket && r.key) {
      const prefix = r.key.includes("/") ? r.key.slice(0, r.key.lastIndexOf("/") + 1) : "";
      navigate(`/buckets/${encodeURIComponent(r.bucket)}?prefix=${encodeURIComponent(prefix)}`);
    } else if (r.type === "user") navigate("/admin/users");
  };

  const icon = (type: string) => {
    if (type === "bucket") return <FolderOpen className="h-4 w-4 text-amber-500" />;
    if (type === "object") return <File className="h-4 w-4 text-muted-foreground" />;
    return <User className="h-4 w-4 text-primary" />;
  };

  return (
    <div ref={ref} className="relative w-full max-w-md">
      <div className="relative">
        <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
        <Input
          placeholder={t("placeholder")}
          className="pl-9 pr-8"
          value={query}
          onChange={(e) => { setQuery(e.target.value); setOpen(true); }}
          onFocus={() => setOpen(true)}
        />
        {query && (
          <button type="button" className="absolute right-2 top-2 text-muted-foreground" onClick={() => setQuery("")}>
            <X className="h-4 w-4" />
          </button>
        )}
      </div>

      {open && debounced.length >= 1 && (
        <div className="absolute z-50 mt-1 w-full rounded-md border bg-popover shadow-lg">
          {search.isLoading ? (
            <p className="p-3 text-sm text-muted-foreground">{t("searching")}</p>
          ) : (search.data?.results ?? []).length === 0 ? (
            <p className="p-3 text-sm text-muted-foreground">{t("noResults", { query: debounced })}</p>
          ) : (
            <ul className="max-h-72 overflow-y-auto py-1">
              {(search.data?.results ?? []).map((r, i) => (
                <li key={`${r.type}-${r.name}-${i}`}>
                  <button
                    type="button"
                    className="flex w-full items-center gap-3 px-3 py-2 text-left text-sm hover:bg-muted"
                    onClick={() => go(r)}
                  >
                    {icon(r.type)}
                    <div className="min-w-0 flex-1">
                      <p className="truncate font-mono">{r.type === "object" ? r.key : r.name}</p>
                      <p className="text-xs text-muted-foreground">
                        {r.type}
                        {r.bucket && r.type === "object" ? ` · ${r.bucket}` : ""}
                        {r.size ? ` · ${formatBytes(r.size, i18n.language)}` : ""}
                        {r.email ? ` · ${r.email}` : ""}
                      </p>
                    </div>
                  </button>
                </li>
              ))}
            </ul>
          )}
          {search.data && search.data.total > (offset + 15) && (
            <button
              type="button"
              className="w-full border-t px-3 py-2 text-xs text-primary hover:bg-muted"
              onClick={() => setOffset((o) => o + 15)}
            >
              {t("loadMore", { remaining: search.data.total - offset - 15 })}
            </button>
          )}
        </div>
      )}
    </div>
  );
}
