import { Card, Space, Spin, Tag, Typography } from 'antd'
import { useTranslation } from 'react-i18next'
import type { SpatialResult } from '../api/client'

const { Text } = Typography

interface Props {
  result: SpatialResult | null
  loading: boolean
  onSelectFeature: (id: string) => void
}

export default function SpatialPanel({ result, loading, onSelectFeature }: Props) {
  const { t } = useTranslation()

  if (!loading && !result) return null

  const featureCard = (
    id: string,
    type: string,
    start: number,
    end: number,
    extra?: React.ReactNode,
  ) => (
    <Card key={`${type}:${id}`} size="small">
      <Space wrap>
        {extra ?? (
          <Tag color={type === 'gene' ? 'blue' : type === 'mRNA' ? 'green' : 'orange'}>{type}</Tag>
        )}
        <Text code style={{ cursor: 'pointer' }} onClick={() => onSelectFeature(id)}>
          {id}
        </Text>
        <Text type="secondary">
          ({start}-{end})
        </Text>
      </Space>
    </Card>
  )

  return (
    <Space orientation="vertical" style={{ width: '100%' }} size="small">
      {loading && (
        <div style={{ textAlign: 'center', padding: 8 }}>
          <Spin size="small" />{' '}
          <Text type="secondary" style={{ fontSize: 12 }}>
            {t('home.spatial.lookingUp')}
          </Text>
        </div>
      )}
      {result && (
        <>
          <Text strong style={{ fontSize: 13 }}>
            {result.features.length > 0
              ? t('home.spatial.overlappingAt', { count: result.features.length, pos: `${result.start}-${result.end}` })
              : t('home.spatial.noOverlap', { pos: `${result.start}-${result.end}` })}
          </Text>
          {result.features.map((f) => featureCard(f.id, f.type, f.start, f.end))}
          {result.features.length > 0 && (
            <Text type="secondary" style={{ fontSize: 11 }}>
              {t('home.transcript.regions.clickId')}
            </Text>
          )}
          {result.upstream &&
            featureCard(
              result.upstream.id,
              result.upstream.type,
              result.upstream.start,
              result.upstream.end,
              <Tag color="default">
                ↑ {t('home.spatial.upstream')} · {result.start - result.upstream.end} bp
              </Tag>,
            )}
          {result.downstream &&
            featureCard(
              result.downstream.id,
              result.downstream.type,
              result.downstream.start,
              result.downstream.end,
              <Tag color="default">
                ↓ {t('home.spatial.downstream')} · {result.downstream.start - result.end} bp
              </Tag>,
            )}
        </>
      )}
    </Space>
  )
}
