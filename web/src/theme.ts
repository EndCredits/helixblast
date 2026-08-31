import type { CSSProperties } from 'react'
import { theme as antdTheme } from 'antd'
import type { ThemeConfig } from 'antd'

export type ResolvedMode = 'light' | 'dark'

// Brand + categorical colors are theme-aware; neutrals are left to AntD's
// algorithms (default/dark) and consumed via theme.useToken() in components.
export const brandLight = '#0e7490'
export const brandDark = '#2dd4bf'

const statusLight = { success: '#059669', warning: '#d97706', error: '#dc2626' }
const statusDark = { success: '#10b981', warning: '#f59e0b', error: '#ef4444' }

// Deep-slate "midnight" base for the dark theme (not pure black, softer than #000)
const darkBgBase = '#0b1220'
const darkTextBase = '#e5e7eb'

export const nucleotideColorsLight: Record<string, string> = {
  A: '#059669', C: '#2563eb', G: '#d97706', T: '#dc2626', U: '#db2777',
}
export const nucleotideColorsDark: Record<string, string> = {
  A: '#34d399', C: '#60a5fa', G: '#fbbf24', T: '#f87171', U: '#f472b6',
}
export const gapColorLight = '#94a3b8'
export const gapColorDark = '#64748b'
export const ambiguousColorLight = '#64748b'
export const ambiguousColorDark = '#94a3b8'

export function getNucleotideColors(mode: ResolvedMode) {
  return mode === 'dark'
    ? { colors: nucleotideColorsDark, gap: gapColorDark, ambiguous: ambiguousColorDark }
    : { colors: nucleotideColorsLight, gap: gapColorLight, ambiguous: ambiguousColorLight }
}

export const FONT_STACK =
  "'Google Sans Flex', 'Noto Sans SC', -apple-system, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif"
export const MONO_STACK =
  "'Fira Code', 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace"

export function getThemeConfig(mode: ResolvedMode): ThemeConfig {
  const dark = mode === 'dark'
  const status = dark ? statusDark : statusLight
  const brand = dark ? brandDark : brandLight

  const base: ThemeConfig = {
    token: {
      colorPrimary: brand,
      colorInfo: brand,
      colorLink: brand,
      colorSuccess: status.success,
      colorWarning: status.warning,
      colorError: status.error,
      fontFamily: FONT_STACK,
      fontFamilyCode: MONO_STACK,
      fontSize: 14,
      borderRadius: 8,
    },
    components: {
      Card: { borderRadiusLG: 12, headerFontSize: 15 },
      Alert: { borderRadiusLG: 10 },
      Tag: { borderRadiusSM: 6 },
    },
  }

  if (dark) {
    base.algorithm = antdTheme.darkAlgorithm
    base.token!.colorBgBase = darkBgBase
    base.token!.colorTextBase = darkTextBase
    base.components!.Layout = { headerHeight: 56, headerPadding: '0 24px' }
  } else {
    base.algorithm = antdTheme.defaultAlgorithm
    base.token!.colorTextBase = '#0f172a'
    base.token!.colorBorder = '#e2e8f0'
    base.token!.colorBorderSecondary = '#eef2f6'
    base.token!.colorBgLayout = '#f1f5f9'
    base.components!.Layout = {
      headerBg: '#ffffff',
      bodyBg: '#f1f5f9',
      headerHeight: 56,
      headerPadding: '0 24px',
    }
    base.components!.Table = { headerBg: '#f8fafc', headerColor: '#334155' }
  }

  return base
}

// Build a code/sequence panel style from resolved tokens so it flips with theme.
export function codePanelStyleFromToken(token: {
  colorFillQuaternary: string
  colorBorder: string
}): CSSProperties {
  return {
    background: token.colorFillQuaternary,
    border: `1px solid ${token.colorBorder}`,
    borderRadius: 8,
    fontFamily: MONO_STACK,
  }
}
