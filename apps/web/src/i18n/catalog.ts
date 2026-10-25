import type {
  LocalizationCatalog,
  LocalizationMessage,
} from "../../../../packages/contracts/src";
import type { Locale } from "./index";
const encoder = new TextEncoder(),
  identifier = /^[A-Za-z0-9_.:-]{1,100}$/,
  parameterName = /^[a-z][a-z0-9_]*$/;
export function wellFormed(value: string) {
  for (let i = 0; i < value.length; i++) {
    const code = value.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = value.charCodeAt(++i);
      if (!(next >= 0xdc00 && next <= 0xdfff)) return false;
    } else if (code >= 0xdc00 && code <= 0xdfff) return false;
  }
  return true;
}
const plain = (value: unknown): value is Record<string, unknown> =>
  !!value &&
  typeof value === "object" &&
  !Array.isArray(value) &&
  (Object.getPrototypeOf(value) === Object.prototype ||
    Object.getPrototypeOf(value) === null);
/** Match the Go contract byte-for-byte; never coerce text, identifiers or counts. */
export function catalogParameters(
  message: Pick<LocalizationMessage, 'parameters'>,
  params: unknown,
) {
  if (params !== undefined && !plain(params)) return;
  const values = (params || {}) as Record<string, unknown>;
  if (Object.keys(values).length !== message.parameters.length) return;
  for (const spec of message.parameters) {
    const value = values[spec.name];
    if (spec.kind === "integer") {
      if (
        typeof value !== "number" ||
        !Number.isSafeInteger(value) ||
        value < 0
      )
        return;
    } else if (
      !["text", "identifier"].includes(spec.kind) ||
      typeof value !== "string" ||
      !value ||
      !wellFormed(value) ||
      encoder.encode(value).length > 512 ||
      /\p{Cc}/u.test(value) ||
      (spec.kind === "identifier" && !identifier.test(value))
    )
      return;
  }
  return values;
}
export function validateCatalog(value: LocalizationCatalog) {
  if (
    !plain(value) ||
    typeof value.version !== "string" ||
    !value.version ||
    value.template_format !== "named-dot-v1" ||
    !Array.isArray(value.messages) ||
    value.messages.length > 10000 ||
    !Array.isArray(value.supported_locales) ||
    !["zh-CN", "en-US"].every((locale) =>
      value.supported_locales.includes(locale),
    ) ||
    !["zh-CN", "en-US"].includes(value.default_locale)
  )
    throw Error("Invalid localization catalog");
  const keys = new Set<string>();
  for (const message of value.messages) {
    if (
      !plain(message) ||
      typeof message.key !== "string" ||
      !/^[a-z][a-z0-9_.-]{0,95}$/.test(message.key) ||
      keys.has(message.key) ||
      !Array.isArray(message.parameters) ||
      message.parameters.length > 100 ||
      !plain(message.texts)
    )
      throw Error("Invalid localization catalog");
    keys.add(message.key);
    const names = new Set<string>();
    for (const spec of message.parameters) {
      if (
        !plain(spec) ||
        typeof spec.name !== "string" ||
        !parameterName.test(spec.name) ||
        names.has(spec.name) ||
        !["text", "identifier", "integer"].includes(spec.kind)
      )
        throw Error("Invalid localization catalog");
      names.add(spec.name);
    }
    for (const locale of ["zh-CN", "en-US"]) {
      const template = message.texts[locale];
      if (
        typeof template !== "string" ||
        !template ||
        !wellFormed(template) ||
        encoder.encode(template).length > 65536
      )
        throw Error("Invalid localization catalog");
      const found = new Set(
        [...template.matchAll(/\{\{\.([a-z][a-z0-9_]*)\}\}/g)].map(
          (match) => match[1],
        ),
      );
      const remaining = template.replace(/\{\{\.([a-z][a-z0-9_]*)\}\}/g, "");
      if (
        remaining.includes("{{") ||
        remaining.includes("}}") ||
        found.size !== names.size ||
        [...found].some((name) => !names.has(name))
      )
        throw Error("Invalid localization catalog");
    }
  }
}
export function formatCatalogMessage(
  message: LocalizationMessage,
  params: unknown,
  locale: Locale,
) {
  const values = catalogParameters(message, params);
  if (!values) return;
  return message.texts[locale]?.replace(
    /\{\{\.([a-z][a-z0-9_]*)\}\}/g,
    (_all, name: string) => String(values[name]),
  );
}
