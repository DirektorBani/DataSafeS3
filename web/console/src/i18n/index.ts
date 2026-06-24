import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import enCommon from "@/locales/en/common.json";
import enNav from "@/locales/en/nav.json";
import enLogin from "@/locales/en/login.json";
import enSetup from "@/locales/en/setup.json";
import enProfile from "@/locales/en/profile.json";
import enDashboard from "@/locales/en/dashboard.json";
import enBuckets from "@/locales/en/buckets.json";
import enBucketDetail from "@/locales/en/bucketDetail.json";
import enUsers from "@/locales/en/users.json";
import enSettings from "@/locales/en/settings.json";
import enTenants from "@/locales/en/tenants.json";
import enPolicy from "@/locales/en/policy.json";
import enUsage from "@/locales/en/usage.json";
import enAccess from "@/locales/en/access.json";
import enFederation from "@/locales/en/federation.json";
import enPublicShare from "@/locales/en/publicShare.json";
import enCluster from "@/locales/en/cluster.json";
import enWebhooks from "@/locales/en/webhooks.json";
import enActivity from "@/locales/en/activity.json";
import enGateway from "@/locales/en/gateway.json";
import enComingSoon from "@/locales/en/comingSoon.json";
import enAdminSettings from "@/locales/en/adminSettings.json";
import enLayout from "@/locales/en/layout.json";
import enSearch from "@/locales/en/search.json";
import enPolicyBuilder from "@/locales/en/policyBuilder.json";
import enNotifications from "@/locales/en/notifications.json";
import ruCommon from "@/locales/ru/common.json";
import ruNav from "@/locales/ru/nav.json";
import ruLogin from "@/locales/ru/login.json";
import ruSetup from "@/locales/ru/setup.json";
import ruProfile from "@/locales/ru/profile.json";
import ruDashboard from "@/locales/ru/dashboard.json";
import ruBuckets from "@/locales/ru/buckets.json";
import ruBucketDetail from "@/locales/ru/bucketDetail.json";
import ruUsers from "@/locales/ru/users.json";
import ruSettings from "@/locales/ru/settings.json";
import ruTenants from "@/locales/ru/tenants.json";
import ruPolicy from "@/locales/ru/policy.json";
import ruUsage from "@/locales/ru/usage.json";
import ruAccess from "@/locales/ru/access.json";
import ruFederation from "@/locales/ru/federation.json";
import ruPublicShare from "@/locales/ru/publicShare.json";
import ruCluster from "@/locales/ru/cluster.json";
import ruWebhooks from "@/locales/ru/webhooks.json";
import ruActivity from "@/locales/ru/activity.json";
import ruGateway from "@/locales/ru/gateway.json";
import ruComingSoon from "@/locales/ru/comingSoon.json";
import ruAdminSettings from "@/locales/ru/adminSettings.json";
import ruLayout from "@/locales/ru/layout.json";
import ruSearch from "@/locales/ru/search.json";
import ruPolicyBuilder from "@/locales/ru/policyBuilder.json";
import ruNotifications from "@/locales/ru/notifications.json";

export const LOCALE_KEY = "datasafe.locale";
export const SUPPORTED_LOCALES = ["en", "ru", "de", "fr"] as const;
export type AppLocale = (typeof SUPPORTED_LOCALES)[number];

const namespaces = [
  "common",
  "nav",
  "login",
  "setup",
  "profile",
  "dashboard",
  "buckets",
  "bucketDetail",
  "users",
  "settings",
  "tenants",
  "policy",
  "usage",
  "access",
  "federation",
  "publicShare",
  "cluster",
  "webhooks",
  "activity",
  "gateway",
  "comingSoon",
  "adminSettings",
  "layout",
  "search",
  "policyBuilder",
  "notifications",
] as const;

export function normalizeLocale(lang: string | undefined | null): AppLocale {
  if (!lang) return "en";
  const base = lang.toLowerCase().split("-")[0];
  if (base === "ru" || base === "de" || base === "fr") return base as AppLocale;
  return "en";
}

export function getStoredLocale(): AppLocale | null {
  const stored = localStorage.getItem(LOCALE_KEY);
  if (!stored) return null;
  return normalizeLocale(stored);
}

export function setStoredLocale(locale: AppLocale) {
  localStorage.setItem(LOCALE_KEY, locale);
}

export function detectInitialLocale(profileLocale?: string | null): AppLocale {
  return normalizeLocale(getStoredLocale() ?? profileLocale ?? navigator.language);
}

const enResources = {
  common: enCommon,
  nav: enNav,
  login: enLogin,
  setup: enSetup,
  profile: enProfile,
  dashboard: enDashboard,
  buckets: enBuckets,
  bucketDetail: enBucketDetail,
  users: enUsers,
  settings: enSettings,
  tenants: enTenants,
  policy: enPolicy,
  usage: enUsage,
  access: enAccess,
  federation: enFederation,
  publicShare: enPublicShare,
  cluster: enCluster,
  webhooks: enWebhooks,
  activity: enActivity,
  gateway: enGateway,
  comingSoon: enComingSoon,
  adminSettings: enAdminSettings,
  layout: enLayout,
  search: enSearch,
  policyBuilder: enPolicyBuilder,
  notifications: enNotifications,
};

void i18n.use(initReactI18next).init({
  resources: {
    en: enResources,
    de: enResources,
    fr: enResources,
    ru: {
      common: ruCommon,
      nav: ruNav,
      login: ruLogin,
      setup: ruSetup,
      profile: ruProfile,
      dashboard: ruDashboard,
      buckets: ruBuckets,
      bucketDetail: ruBucketDetail,
      users: ruUsers,
      settings: ruSettings,
      tenants: ruTenants,
      policy: ruPolicy,
      usage: ruUsage,
      access: ruAccess,
      federation: ruFederation,
      publicShare: ruPublicShare,
      cluster: ruCluster,
      webhooks: ruWebhooks,
      activity: ruActivity,
      gateway: ruGateway,
      comingSoon: ruComingSoon,
      adminSettings: ruAdminSettings,
      layout: ruLayout,
      search: ruSearch,
      policyBuilder: ruPolicyBuilder,
      notifications: ruNotifications,
    },
  },
  lng: detectInitialLocale(),
  fallbackLng: { de: ["en"], fr: ["en"], default: ["en"] },
  defaultNS: "common",
  ns: [...namespaces],
  interpolation: { escapeValue: false },
});

export default i18n;
