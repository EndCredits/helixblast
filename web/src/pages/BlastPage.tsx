import { useCallback, useMemo, useRef, useEffect, useState } from 'react'
import {
  Card,
  Row,
  Col,
  Typography,
  Spin,
  Alert,
  Divider,
  Button,
  Space,
  Tag,
  Collapse,
} from 'antd'
import { App as AntApp } from 'antd'
import { useTranslation } from 'react-i18next'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useDatabases, useJobSSE, useCreateJob, useCancelJob } from '../hooks/useJobs'
import SequenceInput from '../components/SequenceInput'
import ParamPanel from '../components/ParamPanel'
import JobCard from '../components/JobCard'
import ResultsTable from '../components/ResultsTable'
import AlignmentView from '../components/AlignmentView'
import SpatialPanel from '../components/SpatialPanel'
import type { Hit, BlastResult, JobItem, SpatialResult } from '../api/client'
import { fetchSpatial } from '../api/client'
import { loadJobs, saveJobMeta, cacheClear } from '../lib/db'
import i18n from '../i18n'

const { Title, Text } = Typography

function validateFASTA(input: string): string | null {
  const trimmed = input.trim()
  if (!trimmed) return i18n.t('home.validation.empty')
  if (!trimmed.startsWith('>')) return i18n.t('home.validation.noHeader')
  const lines = trimmed.split('\n').filter((l) => l.trim())
  if (lines.length < 2) return i18n.t('home.validation.tooShort')
  return null
}

export default function BlastPage() {
  const { message } = AntApp.useApp()
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()

  const { data: databases = [] } = useDatabases()
  const createJob = useCreateJob()
  const cancelJob = useCancelJob()

  const [fasta, setFasta] = useState('')
  const [program, setProgram] = useState('blastn')
  const [database, setDatabase] = useState<string[]>([])
  const [template, setTemplate] = useState('megablast')
  const [advancedParams, setAdvancedParams] = useState('')
  const [selectedHit, setSelectedHit] = useState<Hit | null>(null)
  const resultsRef = useRef<HTMLDivElement>(null)
  const alignmentRef = useRef<HTMLDivElement>(null)
  const spatialRef = useRef<HTMLDivElement>(null)
  const [spatialResult, setSpatialResult] = useState<SpatialResult | null>(null)
  const [spatialLoading, setSpatialLoading] = useState(false)
  const [savedJobs, setSavedJobs] = useState<JobItem[]>([])

  const selectedJobId = searchParams.get('job')

  const setSelectedJobId = useCallback(
    (id: string | null) => {
      const next = new URLSearchParams(searchParams)
      if (id) {
        next.set('job', id)
      } else {
        next.delete('job')
      }
      setSearchParams(next)
    },
    [searchParams, setSearchParams],
  )

  const reloadLocalJobs = useCallback(() => {
    loadJobs().then((jobs) =>
      setSavedJobs(jobs.sort((a, b) =>
        new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
      )),
    ).catch(() => {})
  }, [])

  useEffect(() => {
    reloadLocalJobs()
  }, [reloadLocalJobs])

  const { data: jobDetail, isLoading: jobLoading, sseState } = useJobSSE(selectedJobId, reloadLocalJobs)

  const fastaError = useMemo(() => {
    if (!fasta.trim()) return null
    return validateFASTA(fasta)
  }, [fasta])

  const handleSelectJob = useCallback((id: string) => {
    setSelectedJobId(id)
    setSelectedHit(null)
    setTimeout(() => {
      resultsRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }, 100)
  }, [setSelectedJobId])

  useEffect(() => {
    if (selectedHit) {
      setTimeout(() => {
        alignmentRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' })
      }, 100)
    }
  }, [selectedHit])

  const resolveHitDb = useCallback((): string | undefined => {
    return selectedHit?.database || (database.length === 1 ? database[0] : undefined)
  }, [selectedHit?.database, database])

  const handleLookupTranscript = useCallback((id: string) => {
    const db = resolveHitDb()
    if (!db) {
      message.error(t('home.errors.dbNotAvailable'))
      return
    }
    navigate(`/transcript?db=${encodeURIComponent(db)}&id=${encodeURIComponent(id)}`)
  }, [resolveHitDb, navigate, message, t])

  const currentDB = useMemo(
    () => databases.find((d) => d.name === (selectedHit?.database || (database.length === 1 ? database[0] : ''))),
    [databases, selectedHit?.database, database],
  )

  // Spatial search is an auxiliary view of BLAST results: whenever a hit on a
  // chromosome database (is_chromosome_db) is selected, resolve the HSP
  // midpoint to overlapping features + flanking genes automatically.
  useEffect(() => {
    if (!selectedHit || !currentDB?.is_chromosome_db) {
      setSpatialResult(null)
      return
    }
    const hsp = selectedHit.alignments[0]
    if (!hsp) {
      setSpatialResult(null)
      return
    }
    const pos = Math.floor((hsp.subject_start + hsp.subject_end) / 2)
    let cancelled = false
    setSpatialLoading(true)
    fetchSpatial(currentDB.name, selectedHit.subject_id, pos)
      .then((r) => {
        if (cancelled) return
        setSpatialResult(r)
        setTimeout(() => {
          spatialRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
        }, 100)
      })
      .catch(() => {
        if (!cancelled) setSpatialResult(null)
      })
      .finally(() => {
        if (!cancelled) setSpatialLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [selectedHit, currentDB])

  const handleSubmit = useCallback(async () => {
    const err = validateFASTA(fasta)
    if (err) {
      message.error(err)
      return
    }
    if (database.length === 0) {
      message.error(t('home.errors.selectDatabase'))
      return
    }

    const advParams: Record<string, string> = {}
    if (template) {
      advParams['task'] = template
    }
    if (advancedParams.trim()) {
      const parts = advancedParams.trim().split(/\s+/)
      for (let i = 0; i < parts.length; i++) {
        if (parts[i].startsWith('-') && i + 1 < parts.length && !parts[i + 1].startsWith('-')) {
          advParams[parts[i].replace(/^-+/, '')] = parts[i + 1]
          i++
        }
      }
    }

    try {
      const res = await createJob.mutateAsync({
        fasta: fasta.trim(),
        program,
        dbs: database,
        advanced_params: advParams,
      })
      const now = new Date().toISOString()
      await saveJobMeta({
        job_id: res.job_id, status: 'queued', queue_pos: res.queue_pos,
        program, database: database.join(','), created_at: now,
      })
      reloadLocalJobs()
      setSelectedJobId(res.job_id)
      message.success(t('home.errors.jobSubmitted', { count: database.length }))
    } catch (e: any) {
      message.error(e.message || t('home.errors.submitFailed'))
    }
  }, [fasta, program, database, template, advancedParams, createJob, reloadLocalJobs, setSelectedJobId])

  const handleCancel = useCallback(
    async (id: string) => {
      try {
        await cancelJob.mutateAsync(id)
        message.info(t('home.errors.cancelRequested'))
      } catch (e: any) {
        message.error(e.message || t('home.errors.cancelFailed'))
      }
    },
    [cancelJob],
  )

  const jobResult = jobDetail?.result as BlastResult | null
  const hits = jobResult?.results || []

  const mergedJobs = useMemo(() => {
    return savedJobs.map((j: any) => {
      const isTerminal = ['success', 'failed', 'cancelled'].includes(j.status)
      return { ...j, _cached: isTerminal }
    })
  }, [savedJobs])

  return (
    <Row gutter={24}>
      {/* Left: Input Panel */}
      <Col xs={24} lg={10}>
        <Card title={<Title level={5} style={{ margin: 0 }}>{t('home.paramPanel.submit')}</Title>}>
          <Space orientation="vertical" style={{ width: '100%' }} size="middle">
            <SequenceInput value={fasta} onChange={setFasta} error={fastaError || undefined} />
            <ParamPanel
              databases={databases}
              program={program}
              database={database}
              template={template}
              advancedParams={advancedParams}
              onProgramChange={setProgram}
              onDatabaseChange={setDatabase}
              onTemplateChange={setTemplate}
              onAdvancedParamsChange={setAdvancedParams}
              onSubmit={handleSubmit}
              loading={createJob.isPending}
            />
          </Space>
        </Card>
      </Col>

      {/* Right: Jobs → Results */}
      <Col xs={24} lg={14}>
        <Card
          title={<Title level={5} style={{ margin: 0 }}>{t('home.jobs.title')}</Title>}
          extra={
            savedJobs.length > 0 && (
              <Button size="small" color="danger" onClick={() => {
                cacheClear().then(() => {
                  setSavedJobs([])
                  setSelectedJobId(null)
                  setSelectedHit(null)
                  message.success(t('home.jobs.cacheCleared'))
                }).catch(() => message.error(t('home.jobs.cacheClearFailed')))
              }}>
                {t('home.jobs.clear', { count: savedJobs.length })}
              </Button>
            )
          }
          style={{ marginBottom: 24 }}
        >
          {mergedJobs.length === 0 ? (
            <Text type="secondary">{t('home.jobs.empty')}</Text>
          ) : (
            <Space orientation="vertical" style={{ width: '100%' }} size="small">
              <div key={mergedJobs[0].job_id}>
                {(mergedJobs[0] as any)._cached && (
                  <Tag color="default" style={{ marginBottom: 4, fontSize: 10 }}>{t('home.jobs.local')}</Tag>
                )}
                <JobCard
                  job={mergedJobs[0]}
                  selected={selectedJobId === mergedJobs[0].job_id}
                  onSelect={handleSelectJob}
                  onCancel={handleCancel}
                />
              </div>
              {mergedJobs.length > 1 && (
                <Collapse
                  ghost
                  size="small"
                  items={[
                    {
                      key: 'older',
                      label: t('home.jobs.olderJobs', { count: mergedJobs.length - 1 }),
                      children: (
                        <Space orientation="vertical" style={{ width: '100%' }} size="small">
                          {mergedJobs.slice(1).map((job) => (
                            <div key={job.job_id}>
                              {(job as any)._cached && (
                                <Tag color="default" style={{ marginBottom: 4, fontSize: 10 }}>{t('home.jobs.local')}</Tag>
                              )}
                              <JobCard
                                job={job}
                                selected={selectedJobId === job.job_id}
                                onSelect={handleSelectJob}
                                onCancel={handleCancel}
                              />
                            </div>
                          ))}
                        </Space>
                      ),
                    },
                  ]}
                />
              )}
            </Space>
          )}
        </Card>

        <div ref={resultsRef}>
          <Card
            title={
              <Space>
                <Title level={5} style={{ margin: 0 }}>{t('home.results.title')}</Title>
                {selectedJobId && <Text code>{selectedJobId}</Text>}
              </Space>
            }
            extra={
              <Space>
                {selectedJobId && sseState !== 'connected' && sseState !== 'disconnected' && (
                  <Tag color="warning">{sseState === 'reconnecting' ? t('home.results.reconnecting') : t('home.results.connecting')}</Tag>
                )}
                {selectedJobId && (
                  <Button size="small" onClick={() => { setSelectedJobId(null); setSelectedHit(null) }}>
                    {t('home.results.close')}
                  </Button>
                )}
              </Space>
            }
          >
            {!selectedJobId && (
              <Text type="secondary">{t('home.jobs.selectHint')}</Text>
            )}

            {jobLoading && <Spin />}

            {jobDetail?.status === 'failed' && (
              <Alert title={t('home.jobs.resultStatus.failed')} description={jobDetail.error} type="error" showIcon />
            )}

            {jobDetail?.status === 'cancelled' && (
              <Alert title={t('home.jobs.resultStatus.cancelled')} type="warning" showIcon />
            )}

            {jobDetail?.status === 'running' && (
              <Alert title={jobDetail.progress || t('home.jobs.resultStatus.running')} type="info" showIcon icon={<Spin />} />
            )}

            {jobDetail?.status === 'queued' && (
              <Alert
                title={t('home.jobs.resultStatus.queued', { pos: jobDetail.queue_pos })}
                type="info"
                showIcon
              />
            )}

            {jobResult?.errors && jobResult.errors.length > 0 && (
              <Alert
                type="warning"
                showIcon
                title={t('home.jobs.resultStatus.dbFailed', { count: jobResult.errors.length })}
                description={
                  <Space orientation="vertical" size={2} style={{ width: '100%' }}>
                    {jobResult.errors.map((e) => (
                      <Text key={e.database} type="danger">
                        <Text strong>{e.database}:</Text> {e.error}
                      </Text>
                    ))}
                  </Space>
                }
                style={{ marginBottom: 12 }}
              />
            )}

            {hits.length > 0 && (
              <>
                <ResultsTable
                  hits={hits}
                  onSelectHit={setSelectedHit}
                />
                {selectedHit && (
                  <div ref={alignmentRef}>
                    <Divider />
                    <AlignmentView
                      hit={selectedHit}
                      onLookupTranscript={currentDB?.is_chromosome_db ? undefined : handleLookupTranscript}
                    />
                    {(spatialLoading || spatialResult) && (
                      <div ref={spatialRef}>
                        <Divider />
                        <SpatialPanel
                          result={spatialResult}
                          loading={spatialLoading}
                          onSelectFeature={handleLookupTranscript}
                        />
                      </div>
                    )}
                  </div>
                )}
              </>
            )}
          </Card>
        </div>
      </Col>
    </Row>
  )
}
