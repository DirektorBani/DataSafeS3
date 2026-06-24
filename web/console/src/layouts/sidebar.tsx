import type { ComponentType } from "react";
import { NavLink } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  Activity,
  BarChart3,
  Database,
  FolderOpen,
  Globe,
  KeyRound,
  LayoutDashboard,
  LogOut,
  Moon,
  Monitor,
  Network,
  Server,
  Settings,
  Shield,
  ShieldCheck,
  Sun,
  Webhook,
  Users,
  Building2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useAuth } from "@/hooks/use-auth";
import { useTheme } from "@/hooks/use-theme";
import { cn } from "@/lib/utils";

function NavItem({
  to,
  label,
  icon: Icon,
  end,
}: {
  to: string;
  label: string;
  icon: ComponentType<{ className?: string }>;
  end?: boolean;
}) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) =>
        cn(
          "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
          isActive
            ? "bg-primary/10 text-primary"
            : "text-muted-foreground hover:bg-accent hover:text-accent-foreground"
        )
      }
    >
      <Icon className="h-4 w-4 shrink-0" />
      {label}
    </NavLink>
  );
}

export function Sidebar() {
  const { t } = useTranslation(["nav", "common"]);
  const { logout, isAdmin, isTenantAdmin, role } = useAuth();
  const { theme, setTheme } = useTheme();
  const showTenants = isAdmin || isTenantAdmin;

  const userNav = [
    { to: "/", label: t("nav:dashboard"), icon: LayoutDashboard, end: true },
    {
      to: "/buckets",
      label: role === "user" ? t("nav:files") : t("nav:buckets"),
      icon: FolderOpen,
    },
    { to: "/keys", label: t("nav:access"), icon: KeyRound },
    { to: "/usage", label: t("nav:usage"), icon: BarChart3 },
    { to: "/profile", label: t("nav:profile"), icon: ShieldCheck },
  ];

  const adminNav = [
    { to: "/admin/users", label: t("nav:users"), icon: Users },
    { to: "/admin/tenants", label: t("nav:tenants"), icon: Building2 },
    { to: "/gateway", label: t("nav:gateway"), icon: Network },
    { to: "/federation", label: t("nav:federation"), icon: Globe },
    { to: "/cluster", label: t("nav:cluster"), icon: Server },
    { to: "/admin/policy", label: t("nav:policies"), icon: Shield },
    { to: "/admin/activity", label: t("nav:activity"), icon: Activity },
    { to: "/admin/webhooks", label: t("nav:webhooks"), icon: Webhook },
    { to: "/admin/settings", label: t("nav:settings"), icon: Settings },
  ];

  return (
    <aside className="flex h-full w-64 flex-col border-r bg-card">
      <div className="flex h-14 items-center gap-2 border-b px-4">
        <Database className="h-5 w-5 text-primary" />
        <div>
          <p className="text-sm font-semibold leading-none">{t("common:brand")}</p>
          <p className="text-xs text-muted-foreground">{t("common:tagline")}</p>
        </div>
      </div>

      <nav className="flex-1 space-y-1 overflow-y-auto p-3">
        <p className="px-3 pb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          {t("nav:section.console")}
        </p>
        {userNav.map((item) => (
          <NavItem key={item.to} {...item} />
        ))}

        {isAdmin && (
          <>
            <Separator className="my-3" />

            <p className="px-3 pb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              {t("nav:section.administration")}
            </p>
            {adminNav.map((item) => (
              <NavItem key={item.to} {...item} />
            ))}
          </>
        )}
        {!isAdmin && showTenants && (
          <>
            <Separator className="my-3" />
            <p className="px-3 pb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              {t("nav:section.tenantAdmin")}
            </p>
            <NavItem to="/admin/tenants" label={t("nav:tenants")} icon={Building2} />
          </>
        )}
      </nav>

      <div className="border-t p-3 space-y-2">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="sm" className="w-full justify-start gap-2">
              {theme === "dark" ? <Moon className="h-4 w-4" /> : theme === "light" ? <Sun className="h-4 w-4" /> : <Monitor className="h-4 w-4" />}
              {t("common:theme.label", { theme })}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="w-48">
            <DropdownMenuLabel>{t("common:theme.appearance")}</DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => setTheme("dark")}>
              <Moon className="mr-2 h-4 w-4" /> {t("common:theme.dark")}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => setTheme("light")}>
              <Sun className="mr-2 h-4 w-4" /> {t("common:theme.light")}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => setTheme("system")}>
              <Monitor className="mr-2 h-4 w-4" /> {t("common:theme.system")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        <Button variant="ghost" size="sm" className="w-full justify-start gap-2" onClick={logout}>
          <LogOut className="h-4 w-4" />
          {t("common:signOut")}
        </Button>
      </div>
    </aside>
  );
}
