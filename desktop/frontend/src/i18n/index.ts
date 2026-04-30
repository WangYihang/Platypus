// i18n bootstrap. Imported once for side effects from main.tsx so the
// global i18next instance is initialised before any component renders.
//
// Two locales ship in this round: en-US (the canonical source) and
// zh-CN. Detection precedence is localStorage → navigator language so
// an explicit Preferences switch wins over a stale Accept-Language
// header, and the chosen language survives reloads.
//
// Translations are split by namespace (one JSON file per UI surface)
// and pre-loaded with the bundle. The current set covers the
// high-frequency entry surfaces (onboarding, login, layout, project
// overview, preferences); other pages still render English literals
// and can be migrated namespace-by-namespace without touching this
// file beyond an import.

import i18n from "i18next";
import LanguageDetector from "i18next-browser-languagedetector";
import { initReactI18next } from "react-i18next";

import enCommon from "./locales/en-US/common.json";
import enLayout from "./locales/en-US/layout.json";
import enLogin from "./locales/en-US/login.json";
import enOnboarding from "./locales/en-US/onboarding.json";
import enPreferences from "./locales/en-US/preferences.json";
import enProjectOverview from "./locales/en-US/projectOverview.json";
import enSecurity from "./locales/en-US/security.json";

import zhCommon from "./locales/zh-CN/common.json";
import zhLayout from "./locales/zh-CN/layout.json";
import zhLogin from "./locales/zh-CN/login.json";
import zhOnboarding from "./locales/zh-CN/onboarding.json";
import zhPreferences from "./locales/zh-CN/preferences.json";
import zhProjectOverview from "./locales/zh-CN/projectOverview.json";
import zhSecurity from "./locales/zh-CN/security.json";

export const SUPPORTED_LANGUAGES = ["en-US", "zh-CN"] as const;
export type SupportedLanguage = (typeof SUPPORTED_LANGUAGES)[number];

export const LANGUAGE_LABELS: Record<SupportedLanguage, string> = {
    "en-US": "English",
    "zh-CN": "简体中文",
};

const LS_KEY = "platypus.lang";

export const i18nReady = i18n
    .use(LanguageDetector)
    .use(initReactI18next)
    .init({
        fallbackLng: "en-US",
        // Keep this list as fully-qualified region tags. Our bundles
        // are keyed by tag, so we don't want i18next to silently
        // narrow "en-US" → "en" before lookup. Conversely we DON'T
        // set `nonExplicitSupportedLngs: true` — combined with
        // regional tags it makes the resolver short-circuit
        // toResolveHierarchy to [], which then makes every t() call
        // fall back to the literal key.
        supportedLngs: [...SUPPORTED_LANGUAGES],
        ns: [
            "common",
            "layout",
            "login",
            "onboarding",
            "preferences",
            "projectOverview",
            "security",
        ],
        defaultNS: "common",
        detection: {
            order: ["localStorage", "navigator"],
            lookupLocalStorage: LS_KEY,
            caches: ["localStorage"],
        },
        resources: {
            "en-US": {
                common: enCommon,
                layout: enLayout,
                login: enLogin,
                onboarding: enOnboarding,
                preferences: enPreferences,
                projectOverview: enProjectOverview,
                security: enSecurity,
            },
            "zh-CN": {
                common: zhCommon,
                layout: zhLayout,
                login: zhLogin,
                onboarding: zhOnboarding,
                preferences: zhPreferences,
                projectOverview: zhProjectOverview,
                security: zhSecurity,
            },
        },
        interpolation: { escapeValue: false },
    });

export default i18n;
