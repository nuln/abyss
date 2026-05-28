import dayjs from "dayjs";
import { createI18n } from "vue-i18n";

import("dayjs/locale/de");
import("dayjs/locale/en");
import("dayjs/locale/es");
import("dayjs/locale/fr");
import("dayjs/locale/it");
import("dayjs/locale/ja");
import("dayjs/locale/ko");
import("dayjs/locale/pt-br");
import("dayjs/locale/ru");
import("dayjs/locale/zh-cn");
import("dayjs/locale/zh-tw");

// All i18n resources specified in the plugin `include` option can be loaded
// at once using the import syntax
import messages from "@intlify/unplugin-vue-i18n/messages";

export function detectLocale() {
  // locale is an RFC 5646 language tag (e.g. en-US, pt-BR, zh-Hant-TW)
  // Use navigator.language (or the first navigator.languages entry) and map
  // to the supported locales. Be permissive with region subtags.
  const raw = (navigator.language || (navigator.languages && navigator.languages[0]) || "en").toLowerCase();
  const parts = raw.split("-");
  const lang = parts[0];
  const region = parts[1] || "";

  // Map language + region to available message keys
  switch (lang) {
    case "es":
      return "es";
    case "en":
      return "en";
    case "it":
      return "it";
    case "fr":
      return "fr";
    case "pt":
      // only pt-br is provided; default pt -> pt-br
      return region === "br" ? "pt-br" : "pt-br";
    case "ja":
      return "ja";
    case "zh":
      // prefer traditional for TW/HK, simplified otherwise
      if (region === "tw" || region === "hk" || raw.includes("hant")) return "zh-tw";
      return "zh-cn";
    case "de":
      return "de";
    case "ru":
      return "ru";
    case "ko":
      return "ko";
    default:
      return "en";
  }
}

// TODO: was this really necessary?
// function removeEmpty(obj: Record<string, any>): void {
//   Object.keys(obj)
//     .filter((k) => obj[k] !== null && obj[k] !== undefined && obj[k] !== "") // Remove undef. and null and empty.string.
//     .reduce(
//       (newObj, k) =>
//         typeof obj[k] === "object"
//           ? Object.assign(newObj, { [k]: removeEmpty(obj[k]) }) // Recurse.
//           : Object.assign(newObj, { [k]: obj[k] }), // Copy value.
//       {}
//     );
// }

export const rtlLanguages: string[] = [];

export const i18n = createI18n({
  // Detect browser language on startup.
  // Language can still be changed later via `setLocale()` (e.g. user setting).
  locale: detectLocale(),
  fallbackLocale: "en",
  messages,
  // expose i18n.global for outside components
  legacy: true,
});

export const isRtl = (locale?: string) => {
  // see below
  // @ts-expect-error incorrect type when legacy
  return rtlLanguages.includes(locale || i18n.global.locale.value);
};

export function setLocale(locale: string) {
  const actualLocale = locale === "auto" || !locale ? detectLocale() : locale;
  dayjs.locale(actualLocale);
  // according to doc u only need .value if legacy: false but they lied
  // https://vue-i18n.intlify.dev/guide/essentials/scope.html#local-scope-1
  // @ts-expect-error incorrect type when legacy
  i18n.global.locale.value = actualLocale;
}

export function setHtmlLocale(locale: string) {
  const html = document.documentElement;
  html.lang = locale;
  if (isRtl(locale)) html.dir = "rtl";
  else html.dir = "ltr";
}

export default i18n;
