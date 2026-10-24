import { useState, type ReactNode } from "react";
import { I18nextProvider } from "react-i18next";
import { Languages } from "lucide-react";
import { getLocale, getLocaleSource, followLocaleDefaults, i18n, setLocale, tr, useLocale } from "./index";
import "./language.css";
export function LanguageProvider({ children }: { children: ReactNode }) {
  return <I18nextProvider i18n={i18n}>{children}</I18nextProvider>;
}
export function LanguageSelector({
  showScope = false,
}: {
  showScope?: boolean;
}) {
  useLocale();
  const [saved, setSaved] = useState(true);
  return (
    <div className="language-control">
      <label className="language-selector" title={tr("preferences.quickHint")}>
        <Languages size={15} aria-hidden="true" />
        <select
          aria-label={tr("common.language")}
          aria-description={tr("preferences.quickHint")}
          value={getLocaleSource()==='explicit'?getLocale():'auto'}
          onKeyDown={(event) => {
            if (event.key === "Escape") event.stopPropagation();
          }}
          onChange={(event) =>
            event.target.value==='auto'?setSaved(followLocaleDefaults()):void setLocale(event.target.value).then(setSaved)
          }
        >
          <option value="auto">{tr('preferences.followDefaults')}</option>
          <option value="zh-CN">简体中文</option>
          <option value="en-US">English</option>
        </select>
      </label>
      {showScope && <small>{tr("preferences.quickHint")}</small>}
      {!saved && (
        <small role="status">{tr("common.preferenceUnavailable")}</small>
      )}
    </div>
  );
}
