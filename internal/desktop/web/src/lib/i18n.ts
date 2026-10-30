import { derived, get, writable } from "svelte/store";

export type AppLocale = "en" | "th";

export const locale = writable<AppLocale>("en");

export function tr(lang: AppLocale, en: string, th: string): string {
  return lang === "th" ? th : en;
}

export function tNow(en: string, th: string): string {
  return get(locale) === "th" ? th : en;
}

export const t = derived(locale, ($locale) => {
  return (en: string, th: string): string => ($locale === "th" ? th : en);
});
