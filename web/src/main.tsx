import React from 'react'
import ReactDOM from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ConfigProvider, theme, App as AntApp } from 'antd'
import enUS from 'antd/locale/en_US'
import zhCN from 'antd/locale/zh_CN'
import { useTranslation } from 'react-i18next'
import './i18n'
import App from './App'

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
    <ConfigProvider
      locale={locale}
      theme={{
        algorithm: theme.defaultAlgorithm,
        token: {
          colorPrimary: '#0e7490',
          colorInfo: '#0e7490',
          colorLink: '#0e7490',
          colorSuccess: '#16a34a',
          colorWarning: '#d97706',
          colorError: '#dc2626',
          borderRadius: 10,
          borderRadiusLG: 20,
          borderRadiusSM: 8,
          colorBgLayout: '#f6f8fa',
          fontFamily: "'Google Sans Flex', 'Noto Sans SC', -apple-system, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif",
          boxShadow: '0 1px 3px rgba(16,24,40,0.06)',
          boxShadowSecondary: '0 8px 24px rgba(16,24,40,0.08)',
        },
        components: {
          Card: {
            borderRadiusLG: 20,
            boxShadow: '0 1px 3px rgba(16,24,40,0.06)',
          },
          Table: {
            headerBg: '#f8fafc',
            headerColor: '#475569',
          },
          Button: {
            borderRadius: 10,
          },
        },
      }}
    >
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
