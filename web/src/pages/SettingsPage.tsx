import { useState, useEffect } from 'react'
import { Card, Typography, Space, Select, Button, Switch, Segmented } from 'antd'
import { App as AntApp } from 'antd'
import { useTranslation } from 'react-i18next'
import { LANGUAGES, saveLang } from '../i18n'
import { getSetting, setSetting, loadJobs, cacheClear } from '../lib/db'
import { useThemeMode, type ThemePreference } from '../themeMode'

const { Title, Text } = Typography

export default function SettingsPage() {
  const { t, i18n } = useTranslation()
  const { message: msg } = AntApp.useApp()
  const { preference, setPreference } = useThemeMode()

  // Local-only states; values are read/written to IndexedDB on demand.
  // Low Resource prompt: dismissedLowResource === true means the prompt was
  // closed by the user. Settings offers a "re-show" toggle (false = show again).
  const [showLowResource, setShowLowResource] = useState(true)
  const [cachedJobs, setCachedJobs] = useState(0)
  const [clearing, setClearing] = useState(false)

  // Load current settings on mount.
  useEffect(() => {
    getSetting('dismissedLowResource').then((v) => {
      setShowLowResource(!v)
    }).catch(() => {})
    loadJobs().then((jobs) => setCachedJobs(jobs.length)).catch(() => {})
  }, [])

  const handleLowResourceToggle = (checked: boolean) => {
    setShowLowResource(checked)
    // checked=true → show prompt → dismissed=false; checked=false → hide → dismissed=true
    setSetting('dismissedLowResource', !checked).catch(() => {})
  }

  const handleClearCache = async () => {
    setClearing(true)
    try {
      await cacheClear()
      setCachedJobs(0)
      msg.success(t('settings.cacheCleared'))
    } catch {
      msg.error(t('settings.cacheClearFailed'))
    } finally {
      setClearing(false)
    }
  }

  return (
    <div style={{ maxWidth: 720, margin: '0 auto', padding: '24px 0' }}>
      <Space orientation="vertical" style={{ width: '100%' }} size="middle">
        <Title level={4} style={{ margin: 0 }}>{t('settings.title')}</Title>

        <Card title={t('settings.language.title')}>
          <Space orientation="vertical" style={{ width: '100%' }}>
            <Text type="secondary">{t('settings.language.desc')}</Text>
            <Select
              value={i18n.language}
              style={{ width: 200 }}
              options={LANGUAGES.map((l) => ({ label: l.label, value: l.key }))}
              onChange={(lang: string) => {
                if (lang === 'en' || lang === 'zh') {
                  saveLang(lang)
                  i18n.changeLanguage(lang)
                }
              }}
            />
          </Space>
        </Card>

        <Card title={t('settings.theme.title')}>
          <Space orientation="vertical" style={{ width: '100%' }}>
            <Text type="secondary">{t('settings.theme.desc')}</Text>
            <Segmented
              value={preference}
              onChange={(v) => setPreference(v as ThemePreference)}
              options={[
                { label: t('settings.theme.system'), value: 'system' },
                { label: t('settings.theme.light'), value: 'light' },
                { label: t('settings.theme.dark'), value: 'dark' },
              ]}
            />
          </Space>
        </Card>

        <Card title={t('settings.notifications.title')}>
          <Space orientation="vertical" style={{ width: '100%' }}>
            <Space style={{ width: '100%', justifyContent: 'space-between' }}>
              <div>
                <Text>{t('settings.notifications.lowResource')}</Text>
                <br />
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {t('settings.notifications.lowResourceDesc')}
                </Text>
              </div>
              <Switch checked={showLowResource} onChange={handleLowResourceToggle} />
            </Space>
          </Space>
        </Card>

        <Card title={t('settings.cache.title')}>
          <Space orientation="vertical" style={{ width: '100%' }} size="small">
            <Text type="secondary">
              {t('settings.cache.desc', { count: cachedJobs })}
            </Text>
            <Space>
              <Button danger loading={clearing} onClick={handleClearCache}>
                {t('settings.cache.clear')}
              </Button>
              <Text type="secondary" style={{ fontSize: 12 }}>
                {t('settings.cache.note')}
              </Text>
            </Space>
          </Space>
        </Card>
      </Space>
    </div>
  )
}
