import { useCallback, useEffect, useState } from 'react'
import {
  Row,
  Col,
  Card,
  Typography,
  Space,
  Select,
  Form,
  Input,
  Button,
  Alert,
  Spin,
  Tag,
} from 'antd'
import { App as AntApp } from 'antd'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router-dom'
import { useDatabases } from '../hooks/useJobs'
import type { TranscriptResult, SpatialResult } from '../api/client'
import { lookupTranscript, fetchSpatial } from '../api/client'
import SequenceText from '../lib/sequenceColor'
import { codePanelStyle } from '../theme'

const { Title, Text } = Typography

export default function TranscriptPage() {
  const { message } = AntApp.useApp()
  const { t } = useTranslation()
  const [searchParams, setSearchParams] = useSearchParams()
  const { data: databases = [] } = useDatabases()

  const dbParam = searchParams.get('db') || ''
  const idParam = searchParams.get('id') || ''
  const chrParam = searchParams.get('chr') || ''
  const posParam = searchParams.get('pos') || ''

  const [inputValue, setInputValue] = useState(idParam)
  const [transcriptResult, setTranscriptResult] = useState<TranscriptResult | null>(null)
  const [spatialResult, setSpatialResult] = useState<SpatialResult | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    setInputValue(idParam)
  }, [idParam])

  useEffect(() => {
    let cancelled = false
    setTranscriptResult(null)
    setSpatialResult(null)

    if (!dbParam) return

    if (idParam) {
      setLoading(true)
      lookupTranscript(dbParam, idParam)
        .then((r) => {
          if (!cancelled) setTranscriptResult(r)
        })
        .catch((e: any) => {
          if (!cancelled) message.error(e.message || t('home.errors.lookupFailed'))
        })
        .finally(() => {
          if (!cancelled) setLoading(false)
        })
    } else if (chrParam && posParam) {
      const pos = parseInt(posParam, 10)
      if (Number.isNaN(pos)) return
      setLoading(true)
      fetchSpatial(dbParam, chrParam, pos)
        .then((r) => {
          if (!cancelled) setSpatialResult(r)
        })
        .catch((e: any) => {
          if (!cancelled) message.error(e.message || t('home.errors.spatialFailed'))
        })
        .finally(() => {
          if (!cancelled) setLoading(false)
        })
    }

    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dbParam, idParam, chrParam, posParam])

  const lookupById = useCallback((id: string) => {
    const next = new URLSearchParams()
    next.set('db', dbParam)
    next.set('id', id)
    setSearchParams(next)
  }, [dbParam, setSearchParams])

  const handleLookup = useCallback(() => {
    const tid = inputValue.trim()
    if (!tid) {
      message.error(t('home.errors.enterTranscriptId'))
      return
    }
    if (!dbParam) {
      message.error(t('home.errors.noDbSelected'))
      return
    }
    lookupById(tid)
  }, [inputValue, dbParam, lookupById, message, t])

  const handleClearSpatial = useCallback(() => {
    const next = new URLSearchParams()
    if (dbParam) next.set('db', dbParam)
    setSearchParams(next)
  }, [dbParam, setSearchParams])

  return (
    <Row gutter={24}>
      <Col xs={24} lg={10}>
        <Card title={<Title level={5} style={{ margin: 0 }}>{t('home.mode.transcript')}</Title>}>
          <Space orientation="vertical" style={{ width: '100%' }} size="small">
            <Form.Item label={t('home.paramPanel.database')} style={{ marginBottom: 0 }}>
              <Select
                value={dbParam || undefined}
                onChange={(v) => setSearchParams(v ? { db: v } : {})}
                options={databases.map((db) => ({
                  label: `${db.name} (${db.type})`,
                  value: db.name,
                }))}
                style={{ width: '100%' }}
                notFoundContent={t('home.transcript.notFound')}
              />
            </Form.Item>
            <Form.Item label={t('home.transcript.label')} style={{ marginBottom: 0 }}>
              <Input
                value={inputValue}
                onChange={(e) => setInputValue(e.target.value)}
                placeholder={t('home.transcript.placeholder')}
                onPressEnter={handleLookup}
              />
            </Form.Item>
            <Button
              color="primary"
              variant="solid"
              onClick={handleLookup}
              loading={loading}
              disabled={!dbParam}
              block
            >
              {dbParam ? t('home.transcript.lookup') : t('home.transcript.selectDbFirst')}
            </Button>
          </Space>
        </Card>
      </Col>

      <Col xs={24} lg={14}>
        <Card title={<Title level={5} style={{ margin: 0 }}>{t('home.results.title')}</Title>}>
          {!dbParam && (
            <Text type="secondary">{t('home.transcript.selectDbFirst')}</Text>
          )}

          {loading && <Spin style={{ display: 'block', margin: '12px auto' }} />}

          {!loading && spatialResult && !transcriptResult && (
            <Space orientation="vertical" style={{ width: '100%' }} size="small">
              <Alert
                title={`${spatialResult.chromosome}:${spatialResult.position}`}
                description={
                  spatialResult.features.length > 0
                    ? t('home.spatial.overlapping', { count: spatialResult.features.length })
                    : t('home.spatial.noFeatures')
                }
                type={spatialResult.features.length > 0 ? 'info' : 'warning'}
                showIcon
              />
              {spatialResult.features.map((f) => (
                <Card key={f.id} size="small">
                  <Space>
                    <Tag color={f.type === 'gene' ? 'blue' : f.type === 'mRNA' ? 'green' : 'orange'}>{f.type}</Tag>
                    <Text code style={{ cursor: 'pointer' }} onClick={() => lookupById(f.id)}>
                      {f.id}
                    </Text>
                    <Text type="secondary">({f.start}-{f.end})</Text>
                  </Space>
                </Card>
              ))}
              {spatialResult.features.length > 0 && (
                <Text type="secondary" style={{ fontSize: 11 }}>{t('home.transcript.regions.clickId')}</Text>
              )}
              {spatialResult.upstream && (
                <Card key={spatialResult.upstream.id} size="small">
                  <Space>
                    <Tag>↑ {t('home.spatial.upstream')} ({spatialResult.position - spatialResult.upstream.end} bp)</Tag>
                    <Text code style={{ cursor: 'pointer' }} onClick={() => lookupById(spatialResult.upstream!.id)}>
                      {spatialResult.upstream.id}
                    </Text>
                    <Text type="secondary">({spatialResult.upstream.start}-{spatialResult.upstream.end})</Text>
                  </Space>
                </Card>
              )}
              {spatialResult.downstream && (
                <Card key={spatialResult.downstream.id} size="small">
                  <Space>
                    <Tag>↓ {t('home.spatial.downstream')} ({spatialResult.downstream.start - spatialResult.position} bp)</Tag>
                    <Text code style={{ cursor: 'pointer' }} onClick={() => lookupById(spatialResult.downstream!.id)}>
                      {spatialResult.downstream.id}
                    </Text>
                    <Text type="secondary">({spatialResult.downstream.start}-{spatialResult.downstream.end})</Text>
                  </Space>
                </Card>
              )}
              <Button size="small" onClick={handleClearSpatial}>{t('home.spatial.clear')}</Button>
            </Space>
          )}

          {!loading && transcriptResult && (
            <Space orientation="vertical" style={{ width: '100%' }} size="middle">
              <Alert
                title={
                  <span>
                    {transcriptResult.chromosome}:{transcriptResult.start}-{transcriptResult.end}{' '}
                    <Tag color="blue">{transcriptResult.type}</Tag>
                    <Tag>{transcriptResult.strand === '+' ? t('home.transcript.strand.forward') : t('home.transcript.strand.reverse')}</Tag>
                  </span>
                }
                description={
                  transcriptResult.related ? (
                    <Space orientation="vertical" size={2}>
                      <span><Text type="secondary">{t('home.transcript.regions.gene')}</Text> <Text code>{transcriptResult.gene_id}</Text></span>
                      {transcriptResult.related.transcripts.length > 0 && (
                        <span>
                          <Text type="secondary">{t('home.transcript.regions.transcripts')}</Text>{' '}
                          {transcriptResult.related.transcripts.map((tid, i) => (
                            <span key={tid}>
                              <Text code style={{ cursor: 'pointer' }}
                                onClick={() => lookupById(tid)}>
                                {tid}
                              </Text>
                              {i < transcriptResult.related!.transcripts.length - 1 && ', '}
                            </span>
                          ))}
                        </span>
                      )}
                      {transcriptResult.related.cdss.length > 0 && (
                        <span><Text type="secondary">{t('home.transcript.regions.cdss')}</Text> {transcriptResult.related.cdss.length} regions</span>
                      )}
                    </Space>
                  ) : (
                    <Text type="secondary">{t('home.transcript.regions.gene')} {transcriptResult.gene_id}</Text>
                  )
                }
                type="success"
                showIcon
              />

              {(() => {
                const upstreamLen = transcriptResult.start - transcriptResult.scan_start
                const up5 = transcriptResult.sequence.slice(0, upstreamLen)
                const up2 = up5.slice(Math.max(0, upstreamLen - 2000))
                const geneSeq = transcriptResult.sequence.slice(upstreamLen)
                const exons = transcriptResult.regions?.exons || []
                const cdss = transcriptResult.regions?.cdss || []

                const extractRegion = (seq: string, regionStart: number, regionEnd: number) =>
                  seq.slice(Math.max(0, regionStart - transcriptResult.start), regionEnd - transcriptResult.start + 1)

                const mrna = exons.map(e => extractRegion(geneSeq, e.start, e.end)).join('')
                const cds = cdss.map(c => extractRegion(geneSeq, c.start, c.end)).join('')

                const regionPanels = [
                  { label: t('home.transcript.regions.up5'), seq: up5, len: up5.length },
                  { label: t('home.transcript.regions.up2'), seq: up2, len: up2.length },
                  { label: t('home.transcript.regions.geneBody', { len: geneSeq.length }), seq: geneSeq, len: geneSeq.length },
                  { label: t('home.transcript.regions.mrna', { len: mrna.length }), seq: mrna, len: mrna.length },
                  { label: t('home.transcript.regions.cds', { len: cds.length }), seq: cds, len: cds.length },
                ].filter(r => r.len > 0)

                return (
                  <Space orientation="vertical" style={{ width: '100%' }} size="small">
                    {regionPanels.map((r) => (
                      <Card
                        key={r.label}
                        size="small"
                        title={<Text strong>{r.label} <Text type="secondary">({r.len} bp)</Text></Text>}
                        extra={
                          <Button size="small" variant="link" onClick={() => {
                            const blob = new Blob([`>${transcriptResult.transcript_id}_${r.label}\n${r.seq.match(/.{1,60}/g)?.join('\n')}`], { type: 'text/plain' })
                            const url = URL.createObjectURL(blob)
                            const a = document.createElement('a')
                            a.href = url
                            a.download = `${transcriptResult.transcript_id}_${r.label.replace(/\s+/g, '_')}.fasta`
                            a.click()
                            URL.revokeObjectURL(url)
                          }}>
                            {t('home.transcript.regions.downloadFasta')}
                          </Button>
                        }
                        style={{ marginBottom: 4 }}
                      >
                        <pre
                          style={{
                            ...codePanelStyle,
                            fontSize: 11,
                            lineHeight: '16px',
                            whiteSpace: 'pre-wrap',
                            wordBreak: 'break-all',
                            margin: 0,
                            maxHeight: 150,
                            overflow: 'auto',
                            padding: 8,
                          }}
                        >
                          <SequenceText seq={r.seq.match(/.{1,60}/g)?.join('\n') || r.seq} />
                        </pre>
                      </Card>
                    ))}
                  </Space>
                )
              })()}
            </Space>
          )}
        </Card>
      </Col>
    </Row>
  )
}
