import { Card, Tag, Button, Typography, Space, Progress } from 'antd'
import { ClockCircleOutlined, CheckCircleOutlined, CloseCircleOutlined, SyncOutlined, MinusCircleOutlined } from '@ant-design/icons'
import type { JobItem } from '../api/client'

const { Text } = Typography

interface Props {
  job: JobItem
  onSelect: (id: string) => void
  onCancel: (id: string) => void
  selected?: boolean
}

const statusMeta: Record<string, { color: string; icon: React.ReactNode }> = {
  pending: { color: 'default', icon: <ClockCircleOutlined /> },
  queued: { color: 'processing', icon: <ClockCircleOutlined /> },
  running: { color: 'processing', icon: <SyncOutlined spin /> },
  success: { color: 'success', icon: <CheckCircleOutlined /> },
  failed: { color: 'error', icon: <CloseCircleOutlined /> },
  cancelled: { color: 'warning', icon: <MinusCircleOutlined /> },
}

export default function JobCard({ job, onSelect, onCancel, selected }: Props) {
  const meta = statusMeta[job.status] || statusMeta.pending
  const isRunning = job.status === 'running'

  return (
    <Card
      size="small"
      hoverable
      onClick={() => onSelect(job.job_id)}
      style={{
        border: selected ? '2px solid #1677ff' : undefined,
        opacity: job.status === 'cancelled' ? 0.6 : 1,
      }}
    >
      <Space direction="vertical" style={{ width: '100%' }} size={4}>
        <Space style={{ justifyContent: 'space-between', width: '100%' }}>
          <Text code>{job.job_id}</Text>
          <Tag icon={meta.icon} color={meta.color}>{job.status}</Tag>
        </Space>

        {job.status === 'queued' && job.queue_pos > 0 && (
          <Text type="secondary">Queue position: #{job.queue_pos}</Text>
        )}

        {isRunning && <Progress percent={99} status="active" showInfo={false} size="small" />}

        <Space style={{ justifyContent: 'space-between', width: '100%' }}>
          <Text type="secondary" style={{ fontSize: 12 }}>
            {job.program} / {job.database}
          </Text>
          <Text type="secondary" style={{ fontSize: 12 }}>
            {new Date(job.created_at).toLocaleTimeString()}
          </Text>
        </Space>

        {['queued', 'running'].includes(job.status) && (
          <Button
            danger
            size="small"
            onClick={(e) => {
              e.stopPropagation()
              onCancel(job.job_id)
            }}
          >
            Cancel
          </Button>
        )}
      </Space>
    </Card>
  )
}
