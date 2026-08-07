import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import en from './locales/en.json'
import zh from './locales/zh.json'

const STORAGE_KEY = 'helixblast_lang'

export const LANGUAGES = [
  { key: 'en', label: 'English' },
  { key: 'zh', label: '中文' },
] as const

export type LangKey = (typeof LANGUAGES)[number]['key']

export function getSavedLang(): LangKey {
  const saved = localStorage.getItem(STORAGE_KEY)
  if (saved === 'en' || saved === 'zh') return saved
  return 'en'
}

export function saveLang(lang: LangKey) {
  localStorage.setItem(STORAGE_KEY, lang)
}

i18n.use(initReactI18next).init({
  resources: {
    en: { translation: en },
    zh: { translation: zh },
  },
  lng: getSavedLang(),
  fallbackLng: 'en',
  interpolation: {
    escapeValue: false, // React already escapes
  },
})

export default i18n
