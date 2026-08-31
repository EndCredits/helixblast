import { useEffect, useState } from 'react'
import { Layout, Alert } from 'antd'
import { Outlet } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import Header from './Header'
import { useHealth } from '../hooks/useJobs'
import { getSetting, setSetting } from '../lib/db'

const { Content } = Layout

export default function AppLayout() {
  const { t } = useTranslation()
  const { data: health } = useHealth()
  const [dismissedLowResource, setDismissedLowResource] = useState(false)

  useEffect(() => {
    getSetting('dismissedLowResource').then((v) => {
      if (v) setDismissedLowResource(true)
    }).catch(() => {})
  }, [])

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header />
      <Content style={{ padding: 24, maxWidth: 1400, margin: '0 auto', width: '100%' }}>
        {health?.status === 'degraded' && !dismissedLowResource && (
          <Alert
            title={t('home.degraded.title')}
            type="warning"
            showIcon
            closable={{
              onClose: () => {
                setDismissedLowResource(true)
                setSetting('dismissedLowResource', true).catch(() => {})
              },
            }}
            style={{ marginBottom: 16, padding: '5px 12px', fontSize: 12 }}
          />
        )}
        <Outlet />
      </Content>
    </Layout>
  )
}
