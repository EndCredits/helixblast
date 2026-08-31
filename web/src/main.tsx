import React from 'react'
import ReactDOM from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ConfigProvider, App as AntApp } from 'antd'
import enUS from 'antd/locale/en_US'
import zhCN from 'antd/locale/zh_CN'
import { useTranslation } from 'react-i18next'
import './i18n'
import App from './App'
import { getThemeConfig } from './theme'
import { ThemeModeProvider, useThemeMode } from './themeMode'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
})

// Applies the AntD theme (light/dark algorithm) and locale from context/i18n.
function ThemedConfigBridge({ children }: { children: React.ReactNode }) {
  const { resolved } = useThemeMode()
  const { i18n } = useTranslation()
  const locale = i18n.language === 'zh' ? zhCN : enUS
  return (
    <ConfigProvider locale={locale} theme={getThemeConfig(resolved)}>
      {children}
    </ConfigProvider>
  )
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <ThemeModeProvider>
        <ThemedConfigBridge>
          <AntApp>
            <App />
          </AntApp>
        </ThemedConfigBridge>
      </ThemeModeProvider>
    </QueryClientProvider>
  </React.StrictMode>,
)
