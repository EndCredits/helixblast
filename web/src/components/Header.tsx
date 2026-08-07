import { Layout, Typography, Tag, Button, Dropdown, Space } from 'antd'
import { GlobalOutlined, SettingOutlined } from '@ant-design/icons'
import { useTranslation } from 'react-i18next'
import { Link, useLocation } from 'react-router-dom'
import { useHealth } from '../hooks/useJobs'
import { LANGUAGES, saveLang } from '../i18n'

const { Header: AntHeader } = Layout
const { Title } = Typography

export default function Header() {
  const { data: health } = useHealth()
  const { t, i18n } = useTranslation()
  const location = useLocation()
  const isSettings = location.pathname === '/settings'

  const statusColor = health?.status === 'healthy' ? 'success' : 'warning'
  const statusText = health?.status === 'healthy' ? t('common.healthy') : t('common.degraded')

  return (
    <div style={{ padding: '12px 24px 0' }}>
      <AntHeader
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '0 24px',
          height: 56,
          lineHeight: '56px',
          background: 'linear-gradient(90deg, #0f766e 0%, #0e7490 55%, #155e75 100%)',
          borderRadius: 16,
          boxShadow: '0 4px 16px rgba(14,116,144,0.18)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <span style={{ fontSize: 20 }}>{'\uD83E\uDDEC'}</span>
          <Title level={4} style={{ color: '#fff', margin: 0 }}>
            {t('common.appName')}
          </Title>
        </div>
        <Space size="middle">
          {health && (
            <>
              <Tag color={statusColor} style={{ borderRadius: 999 }}>
                {statusText}
              </Tag>
              <span style={{ color: 'rgba(255,255,255,0.65)', fontSize: 13 }}>
                {t('header.statusLine', {
                  version: health.version,
                  workers: t('common.workers', { count: health.concurrent_capacity }),
                  storage: health.storage_backend,
                })}
              </span>
            </>
          )}
          <Link to={isSettings ? '/' : '/settings'}>
          <Button
            size="small"
            ghost
            icon={<SettingOutlined />}
            style={{ borderRadius: 999 }}
          />
        </Link>
        <Dropdown
            menu={{
              items: LANGUAGES.map((l) => ({
                key: l.key,
                label: l.label,
                disabled: i18n.language === l.key,
              })),
            onClick: ({ key }) => {
              const lang = key as (typeof LANGUAGES)[number]['key']
              saveLang(lang)
              i18n.changeLanguage(lang)
            },
            selectable: true,
            selectedKeys: [i18n.language],
          }}
          >
            <Button
              size="small"
              ghost
              icon={<GlobalOutlined />}
              style={{ borderRadius: 999 }}
            >
              {i18n.language === 'zh' ? '中文' : 'EN'}
            </Button>
          </Dropdown>
        </Space>
      </AntHeader>
    </div>
  )
}
