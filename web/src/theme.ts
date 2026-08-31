import type { CSSProperties } from 'react'
import type { ThemeConfig } from 'antd'

export const brand = '#0e7490'
export const ink = '#0f172a'
export const inkSecondary = '#334155'
export const inkMuted = '#64748b'

export const surfaceBg = '#ffffff'
export const surfaceMuted = '#f8fafc'
export const layoutBg = '#f1f5f9'
export const borderColor = '#e2e8f0'
export const dividerColor = '#eef2f6'

export const successColor = '#059669'
export const warningColor = '#d97706'
export const errorColor = '#dc2626'

export const FONT_STACK =
  "'Google Sans Flex', 'Noto Sans SC', -apple-system, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif"
export const MONO_STACK =
  "'Fira Code', 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace"

export const nucleotideColors: Record<string, string> = {
  A: successColor,
  C: '#2563eb',
  G: warningColor,
  T: errorColor,
  U: '#db2777',
}
export const gapColor = '#94a3b8'
export const ambiguousColor = inkMuted

export const codePanelStyle: CSSProperties = {
  background: surfaceMuted,
  border: `1px solid ${borderColor}`,
  borderRadius: 8,
  fontFamily: MONO_STACK,
}

export const themeConfig: ThemeConfig = {
  token: {
    colorPrimary: brand,
    colorInfo: brand,
    colorLink: brand,
    colorSuccess: successColor,
    colorWarning: warningColor,
    colorError: errorColor,
    colorTextBase: ink,
    colorBorder: borderColor,
    colorBorderSecondary: dividerColor,
    colorBgLayout: layoutBg,
    fontFamily: FONT_STACK,
    fontFamilyCode: MONO_STACK,
    fontSize: 14,
    borderRadius: 8,
  },
  components: {
    Layout: {
      headerBg: surfaceBg,
      bodyBg: layoutBg,
      headerHeight: 56,
      headerPadding: '0 24px',
    },
    Card: {
      borderRadiusLG: 12,
      headerFontSize: 15,
    },
    Table: {
      headerBg: surfaceMuted,
      headerColor: inkSecondary,
    },
    Alert: {
      borderRadiusLG: 10,
    },
    Tag: {
      borderRadiusSM: 6,
    },
  },
}
