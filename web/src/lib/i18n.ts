import i18next, { type i18n as I18n } from 'i18next'
import { initReactI18next } from 'react-i18next'
import en from '@/locales/en.json'
import zhCN from '@/locales/zh-CN.json'

export type Language = 'zh-CN' | 'en'

export const LANGUAGE_STORAGE_KEY = 'vp-language'
export const languages: readonly Language[] = ['zh-CN', 'en']
export const defaultLanguage: Language = 'zh-CN'

function isLanguage(value: unknown): value is Language {
  return value === 'zh-CN' || value === 'en'
}

export function readStoredLanguage(): Language {
  try {
    const value = localStorage.getItem(LANGUAGE_STORAGE_KEY)
    return isLanguage(value) ? value : defaultLanguage
  } catch {
    return defaultLanguage
  }
}

export function storeLanguage(language: Language): void {
  try {
    localStorage.setItem(LANGUAGE_STORAGE_KEY, language)
  } catch {
    // Storage may be unavailable (private mode); the in-memory choice still applies.
  }
}

/** Creates an isolated i18next instance so each app mount owns its language state. */
export function createI18n(language: Language = readStoredLanguage()): I18n {
  const instance = i18next.createInstance()
  instance.on('languageChanged', (lng) => {
    document.documentElement.lang = lng
  })
  void instance.use(initReactI18next).init({
    resources: {
      'zh-CN': { translation: zhCN },
      en: { translation: en },
    },
    lng: language,
    fallbackLng: defaultLanguage,
    supportedLngs: languages,
    interpolation: { escapeValue: false },
    returnNull: false,
    initAsync: false,
  })
  document.documentElement.lang = language
  return instance
}

export async function changeLanguage(instance: I18n, language: Language): Promise<void> {
  storeLanguage(language)
  await instance.changeLanguage(language)
}
