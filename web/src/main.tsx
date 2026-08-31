import React from 'react'
import ReactDOM from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ConfigProvider, App as AntApp } from 'antd'
import enUS from 'antd/locale/en_US'
import zhCN from 'antd/locale/zh_CN'
import { useTranslation } from 'react-i18next'
import './i18n'
import App from './App'
import { themeConfig } from './theme'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
})

// Sync antd locale with i18n language.
function AntdLocaleBridge({ children }: { children: React.ReactNode }) {
  const { i18n } = useTranslation()
  const locale = i18n.language === 'zh' ? zhCN : enUS
  return (
    <ConfigProvider locale={locale} theme={themeConfig}>
      {children}
    </ConfigProvider>
  )
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <AntdLocaleBridge>
        <AntApp>
          <App />
        </AntApp>
      </AntdLocaleBridge>
    </QueryClientProvider>
  </React.StrictMode>,
)
