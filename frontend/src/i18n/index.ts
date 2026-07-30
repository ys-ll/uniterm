import { computed, ref } from "vue";
import type { Language, Locale } from "../types/settings";
import { SUPPORTED_LOCALES } from "../types/settings";

import zhCN from "./locales/zh-CN.json";
import zhTW from "./locales/zh-TW.json";
import en from "./locales/en.json";
import ja from "./locales/ja.json";
import ko from "./locales/ko.json";
import de from "./locales/de.json";
import es from "./locales/es.json";
import fr from "./locales/fr.json";
import ru from "./locales/ru.json";

const messages: Record<Locale, Record<string, string>> = {
  "zh-CN": zhCN,
  "zh-TW": zhTW,
  en,
  ja,
  ko,
  de,
  es,
  fr,
  ru,
};

const currentLocale = ref<Locale>("en");

export const locale = computed(() => currentLocale.value);

const localePrefixMap: Record<string, Locale> = Object.fromEntries(
  SUPPORTED_LOCALES.filter((loc) => loc !== "zh-CN" && loc !== "zh-TW").map(
    (loc) => [loc, loc],
  ),
);

function resolveLocale(lang: Language): Locale {
  if (lang !== "system") {
    if (SUPPORTED_LOCALES.includes(lang as Locale)) return lang as Locale;
    return "en";
  }

  if (typeof navigator === "undefined" || !navigator.language) return "en";

  const nav = navigator.language.toLowerCase();

  if (nav.startsWith("zh")) {
    if (nav.includes("tw") || nav.includes("hk") || nav.includes("mo"))
      return "zh-TW";
    return "zh-CN";
  }

  for (const [prefix, loc] of Object.entries(localePrefixMap)) {
    if (nav.startsWith(prefix)) return loc;
  }

  return "en";
}

export function setLocale(lang: Language) {
  currentLocale.value = resolveLocale(lang);
}

const missingKeyWarned = new Set<string>();

function translate(
  loc: Locale,
  key: string,
  params?: Record<string, string | number>,
): string {
  const localized = messages[loc]?.[key];
  if (localized !== undefined) {
    if (!params) return localized;
    return localized.replace(/\{(\w+)\}/g, (_, k) =>
      String(params[k] ?? `{${k}}`),
    );
  }

  const fallback = messages["en"]?.[key];
  if (fallback !== undefined) {
    if (!missingKeyWarned.has(key)) {
      missingKeyWarned.add(key);
      // eslint-disable-next-line no-console
      console.warn(
        `[i18n] missing key "${key}" in locale "${loc}", fell back to en`,
      );
    }
    if (!params) return fallback;
    return fallback.replace(/\{(\w+)\}/g, (_, k) =>
      String(params[k] ?? `{${k}}`),
    );
  }

  // Hard miss — never render the raw key path; surface a readable placeholder
  // so a missing key doesn't look like a UI bug to end users.
  if (!missingKeyWarned.has(key)) {
    missingKeyWarned.add(key);
    // eslint-disable-next-line no-console
    console.warn(`[i18n] untranslated key "${key}" — no en fallback either`);
  }
  return `[${key.split(".").pop() || key}]`;
}

export function useI18n() {
  function t(key: string, params?: Record<string, string | number>): string {
    return translate(currentLocale.value, key, params);
  }
  return { t, locale };
}

export function t(
  key: string,
  params?: Record<string, string | number>,
): string {
  return translate(currentLocale.value, key, params);
}
