import { Layout, Typography, Button, Dropdown, Space, Segmented, Grid, Tooltip, theme } from 'antd'
import { GlobalOutlined, SunOutlined, MoonOutlined } from '@ant-design/icons'
import { useTranslation } from 'react-i18next'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useHealth } from '../hooks/useJobs'
import { LANGUAGES, saveLang } from '../i18n'
import { useThemeMode } from '../themeMode'

const { Header: AntHeader } = Layout
const { Text } = Typography

function HelixMark({ color }: { color: string }) {
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M8.5 2.5c0 4.5 7 4.5 7 9.5s-7 5-7 9.5" stroke={color} strokeWidth="1.8" strokeLinecap="round" />
      <path d="M15.5 2.5c0 4.5-7 4.5-7 9.5s7 5 7 9.5" stroke={color} strokeOpacity="0.45" strokeWidth="1.8" strokeLinecap="round" />
      <path d="M9.8 4.8h4.4M9.8 19.2h4.4" stroke={color} strokeOpacity="0.7" strokeWidth="1.4" strokeLinecap="round" />
    </svg>
  )
}

export default function Header() {
  const { data: health } = useHealth()
  const { t, i18n } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()
  const screens = Grid.useBreakpoint()
  const { token } = theme.useToken()
  const { resolved, toggle } = useThemeMode()

  const isSettings = location.pathname === '/settings'
  const isTranscript = location.pathname.startsWith('/transcript')
  const activeKey = isSettings ? 'settings' : isTranscript ? 'transcript' : 'blast'

  const healthy = health?.status !== 'degraded'

  return (
    <AntHeader
      style={{
        position: 'sticky',
        top: 0,
        zIndex: 20,
        borderBottom: `1px solid ${token.colorBorderSecondary}`,
        lineHeight: 'normal',
      }}
    >
      <div
        style={{
          maxWidth: 1400,
          margin: '0 auto',
          height: '100%',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 16,
        }}
      >
        <Link to="/blast" style={{ display: 'flex', alignItems: 'center', gap: 10, flexShrink: 0 }}>
          <HelixMark color={token.colorPrimary} />
          <span style={{ fontSize: 16, fontWeight: 600, color: token.colorText, letterSpacing: -0.2 }}>
            {t('common.appName')}
          </span>
        </Link>

        <Space size={screens.md ? 20 : 10}>
          <Segmented
            size="small"
            value={activeKey}
            options={[
              { value: 'blast', label: t('nav.blast') },
              { value: 'transcript', label: t('nav.transcript') },
              { value: 'settings', label: t('nav.settings') },
            ]}
            onChange={(v) => navigate(`/${v}`)}
          />
          {health && (
            <Space size={6}>
              <span
                aria-hidden
                style={{
                  width: 8,
                  height: 8,
                  borderRadius: '50%',
                  background: healthy ? token.colorSuccess : token.colorWarning,
                  display: 'inline-block',
                  flexShrink: 0,
                }}
              />
              <Text style={{ fontSize: 12, color: token.colorTextTertiary, whiteSpace: 'nowrap' }}>
                {healthy ? t('common.healthy') : t('common.degraded')}
                {screens.lg &&
                  ` · ${t('header.statusLine', {
                    version: health.version,
                    workers: t('common.workers', { count: health.concurrent_capacity }),
                    storage: health.storage_backend,
                  })}`}
              </Text>
            </Space>
          )}
          <Tooltip title={t('settings.theme.toggle')}>
            <Button
              size="small"
              type="text"
              aria-label={t('settings.theme.toggle')}
              icon={resolved === 'dark' ? <SunOutlined /> : <MoonOutlined />}
              onClick={toggle}
            />
          </Tooltip>
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
            <Button size="small" type="text" icon={<GlobalOutlined />}>
              {i18n.language === 'zh' ? '中文' : 'EN'}
            </Button>
          </Dropdown>
        </Space>
      </div>
    </AntHeader>
  )
}
