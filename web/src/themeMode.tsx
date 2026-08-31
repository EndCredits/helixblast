import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react'
import type { ReactNode } from 'react'
import { theme as antdTheme } from 'antd'
import {
  getNucleotideColors,
  codePanelStyleFromToken,
  type ResolvedMode,
} from './theme'

export type ThemePreference = 'system' | 'light' | 'dark'

const STORAGE_KEY = 'helixblast_theme'

export function getStoredTheme(): ThemePreference {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'light' || v === 'dark' || v === 'system') return v
  } catch {
    /* localStorage unavailable */
  }
  return 'system'
}

export function storeTheme(pref: ThemePreference) {
  try {
    localStorage.setItem(STORAGE_KEY, pref)
  } catch {
    /* ignore */
  }
}

function systemPrefersDark(): boolean {
  return typeof window !== 'undefined' && !!window.matchMedia
    && window.matchMedia('(prefers-color-scheme: dark)').matches
}

function resolve(preference: ThemePreference): ResolvedMode {
  if (preference === 'system') return systemPrefersDark() ? 'dark' : 'light'
  return preference
}

interface ThemeCtx {
  preference: ThemePreference
  resolved: ResolvedMode
  setPreference: (p: ThemePreference) => void
  toggle: () => void
}

const Ctx = createContext<ThemeCtx | null>(null)

export function ThemeModeProvider({ children }: { children: ReactNode }) {
  const [preference, setPref] = useState<ThemePreference>(() => getStoredTheme())
  const [resolved, setResolved] = useState<ResolvedMode>(() => resolve(preference))

  useEffect(() => {
    setResolved(resolve(preference))
    if (preference !== 'system') return
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => setResolved(resolve('system'))
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [preference])

  useEffect(() => {
    const root = document.documentElement
    root.dataset.theme = resolved
    root.style.colorScheme = resolved
  }, [resolved])

  const setPreference = useCallback((p: ThemePreference) => {
    setPref(p)
    storeTheme(p)
  }, [])

  const toggle = useCallback(() => {
    setPreference(resolved === 'dark' ? 'light' : 'dark')
  }, [resolved, setPreference])

  const value = useMemo<ThemeCtx>(
    () => ({ preference, resolved, setPreference, toggle }),
    [preference, resolved, setPreference, toggle],
  )

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useThemeMode(): ThemeCtx {
  const ctx = useContext(Ctx)
  if (!ctx) throw new Error('useThemeMode must be used within ThemeModeProvider')
  return ctx
}

export function useNucleotideColors() {
  const { resolved } = useThemeMode()
  return useMemo(() => getNucleotideColors(resolved), [resolved])
}

export function useCodePanelStyle() {
  const { token } = antdTheme.useToken()
  return useMemo(
    () =>
      codePanelStyleFromToken({
        colorFillQuaternary: token.colorFillQuaternary,
        colorBorder: token.colorBorder,
      }),
    [token.colorFillQuaternary, token.colorBorder],
  )
}
