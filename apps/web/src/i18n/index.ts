import { validateCatalog, formatCatalogMessage, wellFormed } from "./catalog";
import { createInstance, type TOptions } from "i18next";
import { initReactI18next, useTranslation } from "react-i18next";
import type {
  LocalizationCatalog,
  LocalizationMessage,
} from "../../../../packages/contracts/src";
import {zh,en} from "./locales/resources";
import {displaySourceDefinitions} from "../../../../packages/contracts/src/display-sources";
// These approved keys require complete source proof in renderDisplayDiagnostic.
// Publishing their text in the server catalog does not create generic aliases.
const sourceBoundDisplayKeys = new Set<string>(displaySourceDefinitions.map(definition => definition.key));
// Older clients must also refuse a newer catalog's unknown reserved-domain key.
const isSourceBoundDisplayKey = (key: string) => sourceBoundDisplayKeys.has(key) || key.startsWith('display.tool.');
export type Locale = "zh-CN" | "en-US";
type Sources = {
  explicit?: unknown;
  account?: unknown;
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
  const account = normalizeLocale(sources.account);
  if (account) return { locale: account, source: "account" as const };
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
type LocaleSource = "explicit" | "account" | "organization" | "accept" | "default";
let source: LocaleSource = "default";
let manualLocale: Locale | undefined;
let savedAccount: unknown;
let savedOrganization: unknown;
export const getLocaleSource = () => source;
function resolveCurrentDefaults() {
  const resolved=resolveLocale({explicit:manualLocale,account:savedAccount,organization:savedOrganization,accept:typeof navigator==='undefined'?[]:navigator.languages});
  source=resolved.source;
  if(typeof document!=='undefined'){
    document.documentElement.lang=resolved.locale;
    document.documentElement.dir='ltr';
  }
  void i18n.changeLanguage(resolved.locale);
  i18n.emit('loaded',{});
  return resolved;
}
/** Saved defaults are presentation data bound to the currently verified UI
 * identity. Call with empty values before an account/org scope changes. */
export function applySavedLocaleDefaults(account?:unknown,organization?:unknown){
  savedAccount=account;savedOrganization=organization;
  return resolveCurrentDefaults();
}
export function followLocaleDefaults(){
  manualLocale=undefined;let saved=true;
  try{localStorage.removeItem('suma-locale-v1');}catch{saved=false;}
  resolveCurrentDefaults();return saved;
}
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
  manualLocale=locale;
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
/** The quick selector remains local; verified account/org defaults are loaded
 * separately after authentication and never substituted for failed reads. */
export function initializeBrowserLocale(organization?: unknown) {
  let explicit: unknown;
  try {
    explicit = localStorage.getItem("suma-locale-v1");
  } catch {
    /* Browser storage may be unavailable; continue negotiation. */
  }
  manualLocale=normalizeLocale(explicit);
  savedAccount=undefined;savedOrganization=organization;
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
/** A source-bound renderer supplies its generated parameter contract. A stale
 * or incompatible catalog cannot widen that contract. */
export function formatServerCatalogMessage(key: string, parameters: LocalizationMessage['parameters'], params: unknown) {
  const message = catalog?.messages.find(message => message.key === key);
  if (!message || message.parameters.length !== parameters.length
    || parameters.some(spec => !message.parameters.some(item => item.name === spec.name && item.kind === spec.kind))) return;
  return formatCatalogMessage(message, params, getLocale());
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
  // Only an actual local UiError instance carries a trusted client identity.
  // JSON/server presentation fields cannot manufacture this provenance.
  if (item?.localDiagnostic instanceof UiError)
    return renderDiagnostic(item.localDiagnostic);
  if (typeof item?.message_key === "string") {
    if (isSourceBoundDisplayKey(item.message_key)) return raw;
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
      !isSourceBoundDisplayKey(message.key) && !message.parameters.length && Object.values(message.texts).includes(raw),
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
/** Native execution messages are evidence. Accept a display identity only when
 * its known canonical English template reproduces that complete raw message. */
export function renderNativeDiagnostic(source: string, key: unknown, params: unknown): {text:string;localized:boolean} {
  const fallback={text:source,localized:false};
  if(typeof key!=="string" || !key.startsWith("native."))return fallback;
  const definition=catalog?.messages.find(message=>message.key===key);
  if(!definition || formatCatalogMessage(definition,params,"en-US")!==source)return fallback;
  const text=formatCatalogMessage(definition,params,getLocale());
  return text===undefined ? fallback : {text,localized:true};
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
