import { validateCatalog, formatCatalogMessage, wellFormed } from "./catalog";
import { createInstance, type TOptions } from "i18next";
import { initReactI18next, useTranslation } from "react-i18next";
import type {
  LocalizationCatalog,
  LocalizationMessage,
} from "../../../../packages/contracts/src";
import zh from "./locales/zh-CN.json";
import en from "./locales/en-US.json";
export type Locale = "zh-CN" | "en-US";
type Sources = {
  explicit?: unknown;
  organization?: unknown;
  accept?: readonly string[];
};
export function normalizeLocale(value: unknown): Locale | undefined {
  if (
    typeof value !== "string" ||
    !wellFormed(value) ||
    new TextEncoder().encode(value).length > 128 ||
    /\p{Cc}/u.test(value)
  )
    return;
  try {
    const tag = new Intl.Locale(value.trim().replaceAll("_", "-")).maximize();
    if (tag.language === "zh" && tag.script === "Hans") return "zh-CN";
    if (tag.language === "en" && tag.script === "Latn") return "en-US";
  } catch {
    /* Unsupported or malformed BCP47 input falls through to the next source. */
  }
}

export function resolveLocale(sources: Sources) {
  const explicit = normalizeLocale(sources.explicit);
  if (explicit) return { locale: explicit, source: "explicit" as const };
  const organization = normalizeLocale(sources.organization);
  if (organization)
    return { locale: organization, source: "organization" as const };
  for (const value of sources.accept || []) {
    const locale = normalizeLocale(value);
    if (locale) return { locale, source: "accept" as const };
  }
  return { locale: "zh-CN" as Locale, source: "default" as const };
}
export const i18n = createInstance();
void i18n.use(initReactI18next).init({
  resources: { "zh-CN": { translation: zh }, "en-US": { translation: en } },
  lng: "zh-CN",
  supportedLngs: ["zh-CN", "en-US"],
  fallbackLng: "zh-CN",
  load: "currentOnly",
  keySeparator: false,
  initAsync: false,
  returnNull: false,
  returnEmptyString: false,
  interpolation: { escapeValue: false },
  react: { useSuspense: false, bindI18n: "languageChanged loaded" },
});
let source: "explicit" | "organization" | "accept" | "default" = "default";
export const getLocale = (): Locale => {
  const locale = i18n.resolvedLanguage || i18n.language;
  return locale === "zh-CN" || locale === "en-US"
    ? locale
    : normalizeLocale(locale) || "zh-CN";
};
export const tr = (key: string, params?: Record<string, unknown>) =>
  String(i18n.t(key, params as TOptions));
export function useLocale() {
  useTranslation();
  return getLocale();
}
export async function setLocale(
  value: string,
  options: { persist?: boolean } = {},
) {
  const locale = normalizeLocale(value);
  if (!locale) throw Error("Unsupported locale");
  let saved = true;
  source = "explicit";
  if (options.persist !== false) {
    try {
      localStorage.setItem("suma-locale-v1", locale);
    } catch {
      saved = false;
    }
  }
  if (typeof document !== "undefined") {
    document.documentElement.lang = locale;
    document.documentElement.dir = "ltr";
  }
  await i18n.changeLanguage(locale);
  return saved;
}
/** Browser preference is deliberately local. No account/org persistence is claimed. */
export function initializeBrowserLocale(organization?: unknown) {
  let explicit: unknown;
  try {
    explicit = localStorage.getItem("suma-locale-v1");
  } catch {
    /* Browser storage may be unavailable; continue negotiation. */
  }
  const resolved = resolveLocale({
    explicit,
    organization,
    accept: typeof navigator !== "undefined" ? navigator.languages : [],
  });
  source = resolved.source;
  if (typeof document !== "undefined") {
    document.documentElement.lang = resolved.locale;
    document.documentElement.dir = "ltr";
  }
  void i18n.changeLanguage(resolved.locale);
  return resolved;
}
export const languageHeaders = () => ({
  "Accept-Language": getLocale(),
  ...(source === "explicit" ? { "X-Suma-Language": getLocale() } : {}),
});
export const formatNumber = (
  value: number | bigint,
  options?: Intl.NumberFormatOptions,
) => new Intl.NumberFormat(getLocale(), options).format(value);
export function formatDateTime(
  value: string | number | Date | undefined,
  options?: Intl.DateTimeFormatOptions,
) {
  if (value === undefined || value === "") return tr("common.unknownTime");
  const date = value instanceof Date ? value : new Date(value);
  return Number.isNaN(date.getTime())
    ? tr("common.invalidTime")
    : new Intl.DateTimeFormat(getLocale(), options).format(date);
}
export const formatRelativeTime = (
  value: number,
  unit: Intl.RelativeTimeFormatUnit,
) =>
  new Intl.RelativeTimeFormat(getLocale(), { numeric: "auto" }).format(
    value,
    unit,
  );
let catalog: LocalizationCatalog | undefined;
export function setServerCatalog(value: LocalizationCatalog) {
  validateCatalog(value);
  catalog = structuredClone(value);
  i18n.emit("loaded", {});
}
/** Only stable keys + validated params, or complete parameter-free source matches,
 * are translated. User content, identifiers, tool output and unknown errors stay raw. */
export function renderDiagnostic(value: unknown): string {
  const item =
    value && typeof value === "object"
      ? (value as Record<string, unknown>)
      : undefined;
  const raw =
    typeof value === "string"
      ? value
      : typeof item?.message === "string"
        ? item.message
        : String(value ?? "");
  if (value instanceof UiError)
    return tr(
      value.ui_key,
      value.ui_key === "pools.validation.integer_range"
        ? {
            ...value.ui_params,
            label: renderDiagnostic(value.ui_params?.label),
          }
        : value.ui_params,
    );
  if (typeof item?.message_key === "string") {
    const message = catalog?.messages.find(
      (message) => message.key === item.message_key,
    );
    if (message) {
      const result = formatCatalogMessage(
        message,
        item?.message_params,
        getLocale(),
      );
      if (result !== undefined) return result;
    }
    return raw;
  }
  const exact = catalog?.messages.find(
    (message) =>
      !message.parameters.length && Object.values(message.texts).includes(raw),
  );
  if (exact) return exact.texts[getLocale()] || raw;
  const entries = Object.entries(zh),
    match = entries.find(
      ([key, text]) =>
        !text.includes("{{") &&
        (text === raw || (en as Record<string, string>)[key] === raw),
    );
  return match ? tr(match[0]) : raw;
}
export class UiError extends Error {
  constructor(
    public ui_key: string,
    public ui_params?: Record<string, unknown>,
  ) {
    super(tr(ui_key, ui_params));
  }
}
export async function loadServerCatalog() {
  try {
    const response = await fetch("/api/v1/localization/catalog", {
      credentials: "same-origin",
      headers: languageHeaders(),
    });
    if (!response.ok) return false;
    setServerCatalog(await response.json());
    return true;
  } catch {
    return false;
  }
}

export const roleLabel = (role?: string) =>
  role &&
  ["owner", "admin", "member", "developer", "reviewer", "viewer"].includes(role)
    ? tr("role." + role)
    : role || tr("role.member");
