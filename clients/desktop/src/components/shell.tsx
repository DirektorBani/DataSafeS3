import type { ComponentType, ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Activity, ArrowLeftRight, Database, FolderOpen, LogOut, Moon, Monitor, Settings, Sun, Users } from "lucide-react";
import { Button } from "@datasafe/ui/ui/button";
import { Separator } from "@datasafe/ui/ui/separator";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@datasafe/ui/ui/dropdown-menu";
import { cn } from "@datasafe/ui/cn";
import { setDesktopLocale, type DesktopLocale } from "../i18n";

export type DesktopView = "profiles" | "buckets" | "transfers" | "diagnostics" | "settings";
export type DesktopTheme = "light" | "dark" | "system";

function NavItem(props: {
  label: string;
  icon: ComponentType<{ className?: string }>;
  active: boolean;
  onClick: () => void;
}) {
  const Icon = props.icon;
  return (
    <button
      type="button"
      onClick={props.onClick}
      className={cn(
        "flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
        props.active ? "bg-primary/10 text-primary" : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
      )}
    >
      <Icon className="h-4 w-4 shrink-0" />
      {props.label}
    </button>
  );
}

export function DesktopShell(props: {
  view: DesktopView;
  onView: (view: DesktopView) => void;
  theme: DesktopTheme;
  onTheme: (theme: DesktopTheme) => void;
  version: string;
  signedIn: boolean;
  onSignOut: () => void;
  banner?: ReactNode;
  children: ReactNode;
}) {
  const { t, i18n } = useTranslation(["shell", "common", "nav"]);
  const locale: DesktopLocale = i18n.language === "ru" ? "ru" : "en";
  const themeIcon = props.theme === "dark" ? Moon : props.theme === "light" ? Sun : Monitor;

  return (
    <div className="flex h-screen overflow-hidden">
      <aside className="flex h-full w-64 flex-col border-r bg-card">
        <div className="flex h-14 items-center gap-2 border-b px-4">
          <Database className="h-5 w-5 text-primary" />
          <div>
            <p className="text-sm font-semibold leading-none">{t("common:brand")}</p>
            <p className="text-xs text-muted-foreground">{t("common:tagline")}</p>
          </div>
        </div>
        <nav className="flex-1 space-y-1 overflow-y-auto p-3">
          <p className="px-3 pb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t("nav:section.console")}</p>
          <NavItem label={t("profiles")} icon={Users} active={props.view === "profiles"} onClick={() => props.onView("profiles")} />
          <NavItem label={t("nav:buckets")} icon={FolderOpen} active={props.view === "buckets"} onClick={() => props.onView("buckets")} />
          <NavItem label={t("transfers")} icon={ArrowLeftRight} active={props.view === "transfers"} onClick={() => props.onView("transfers")} />
          <NavItem label={t("diagnostics")} icon={Activity} active={props.view === "diagnostics"} onClick={() => props.onView("diagnostics")} />
          <Separator className="my-3" />
          <NavItem label={t("nav:settings")} icon={Settings} active={props.view === "settings"} onClick={() => props.onView("settings")} />
        </nav>
        <div className="space-y-2 border-t p-3">
          <p className="px-1 text-xs text-muted-foreground">{t("common:version", { version: props.version })}</p>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" size="sm" className="w-full justify-start gap-2">
                {themeIcon === Moon ? <Moon className="h-4 w-4" /> : themeIcon === Sun ? <Sun className="h-4 w-4" /> : <Monitor className="h-4 w-4" />}
                {t("common:theme.label", { theme: props.theme })}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="w-48">
              <DropdownMenuLabel>{t("common:theme.appearance")}</DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => props.onTheme("dark")}>
                <Moon className="mr-2 h-4 w-4" /> {t("common:theme.dark")}
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => props.onTheme("light")}>
                <Sun className="mr-2 h-4 w-4" /> {t("common:theme.light")}
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => props.onTheme("system")}>
                <Monitor className="mr-2 h-4 w-4" /> {t("common:theme.system")}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
          <Button variant="ghost" size="sm" className="w-full justify-start gap-2" disabled={!props.signedIn} onClick={props.onSignOut}>
            <LogOut className="h-4 w-4" />
            {t("common:signOut")}
          </Button>
        </div>
      </aside>
      <main className="flex-1 overflow-y-auto">
        {props.banner}
        <div className="flex items-center justify-end gap-4 border-b px-6 py-3 lg:px-8">
          <div className="inline-flex items-center rounded-md border bg-background p-0.5 text-xs font-medium" role="group" aria-label={t("common:languageSwitcher.ariaLabel")}>
            {(["en", "ru"] as const).map((code, index) => (
              <span key={code} className="inline-flex items-center">
                {index > 0 ? <span className="px-0.5 text-muted-foreground">|</span> : null}
                <button
                  type="button"
                  onClick={() => setDesktopLocale(code)}
                  className={cn("rounded px-2 py-1 transition-colors", locale === code ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:text-foreground")}
                  aria-pressed={locale === code}
                >
                  {code.toUpperCase()}
                </button>
              </span>
            ))}
          </div>
        </div>
        <div className="mx-auto max-w-6xl p-6 lg:p-8">{props.children}</div>
      </main>
    </div>
  );
}
