import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export function NotificationBell() {
  const { t } = useTranslation("notifications");
  const queryClient = useQueryClient();
  const notifications = useQuery({
    queryKey: ["notifications"],
    queryFn: async () => api.listNotifications(),
    refetchInterval: 60_000,
  });

  const markRead = useMutation({
    mutationFn: (id: string) => api.markNotificationRead(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["notifications"] }),
  });

  const markAllRead = useMutation({
    mutationFn: () => api.markAllNotificationsRead(),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["notifications"] }),
  });

  const unread = notifications.data?.unread ?? 0;
  const items = notifications.data?.notifications ?? [];

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" className="relative" aria-label={t("title")}>
          <Bell className="h-4 w-4" />
          {unread > 0 && (
            <span className="absolute right-1 top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[10px] font-medium text-primary-foreground">
              {unread > 9 ? "9+" : unread}
            </span>
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-80">
        <div className="flex items-center justify-between px-2 py-1.5">
          <DropdownMenuLabel className="p-0">{t("title")}</DropdownMenuLabel>
          {unread > 0 && (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 text-xs"
              disabled={markAllRead.isPending}
              onClick={() => markAllRead.mutate()}
            >
              {t("markAllRead")}
            </Button>
          )}
        </div>
        <DropdownMenuSeparator />
        {items.length === 0 && (
          <div className="px-2 py-4 text-center text-sm text-muted-foreground">{t("empty")}</div>
        )}
        {items.map((n) => (
          <DropdownMenuItem key={n.id} className="flex flex-col items-start gap-1 p-0" asChild>
            {n.link ? (
              <Link
                to={n.link}
                className="w-full px-2 py-2"
                onClick={() => {
                  if (!n.read_at) markRead.mutate(n.id);
                }}
              >
                <span className={`text-sm ${n.read_at ? "text-muted-foreground" : "font-medium"}`}>{n.title}</span>
                {n.body && <span className="text-xs text-muted-foreground line-clamp-2">{n.body}</span>}
              </Link>
            ) : (
              <button
                type="button"
                className="w-full px-2 py-2 text-left"
                onClick={() => {
                  if (!n.read_at) markRead.mutate(n.id);
                }}
              >
                <span className={`text-sm ${n.read_at ? "text-muted-foreground" : "font-medium"}`}>{n.title}</span>
                {n.body && <span className="text-xs text-muted-foreground line-clamp-2">{n.body}</span>}
              </button>
            )}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
