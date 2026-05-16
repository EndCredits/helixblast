import { Layout, Typography, Tag } from 'antd'
import { useHealth } from '../hooks/useJobs'

const { Header: AntHeader } = Layout
const { Title } = Typography

export default function Header() {
  const { data: health } = useHealth()

  const statusColor = health?.status === 'healthy' ? 'success' : 'warning'
  const statusText = health?.status === 'healthy' ? 'Healthy' : 'Degraded'

  return (
    <AntHeader
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '0 24px',
        background: '#001529',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <span style={{ fontSize: 20 }}>{'\uD83E\uDDEC'}</span>
        <Title level={4} style={{ color: '#fff', margin: 0 }}>
          HelixBLAST
        </Title>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        {health && (
          <>
            <Tag color={statusColor}>{statusText}</Tag>
            <span style={{ color: 'rgba(255,255,255,0.65)', fontSize: 13 }}>
              v{health.version} | {health.concurrent_capacity} workers | {health.storage_backend}
            </span>
          </>
        )}
      </div>
    </AntHeader>
  )
}
