import { useCallback, useMemo, useRef, useEffect, useState } from 'react'
import {
  Layout,
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
  Radio,
  Input,
  Select,
  Form,
  message,
} from 'antd'
import {
  useHealth,
  useDatabases,
  useJobs,
  useJobSSE,
  useCreateJob,
  useCancelJob,
} from '../hooks/useJobs'
import Header from '../components/Header'
import SequenceInput from '../components/SequenceInput'
import ParamPanel from '../components/ParamPanel'
import JobCard from '../components/JobCard'
import ResultsTable from '../components/ResultsTable'
import AlignmentView from '../components/AlignmentView'
import type { Hit, BlastResult, TranscriptResult } from '../api/client'
import { lookupTranscript } from '../api/client'

const { Content } = Layout
const { Title, Text } = Typography

function validateFASTA(input: string): string | null {
  const trimmed = input.trim()
  if (!trimmed) return 'Sequence input is required'
  if (!trimmed.startsWith('>')) return 'Input must start with a FASTA header (>)'
  const lines = trimmed.split('\n').filter((l) => l.trim())
  if (lines.length < 2) return 'FASTA must contain a header and at least one sequence line'
  return null
}

type QueryMode = 'blast' | 'transcript'

export default function HomePage() {
  const { data: health } = useHealth()
  const { data: databases = [] } = useDatabases()
  const { data: jobs = [] } = useJobs()
  const createJob = useCreateJob()
  const cancelJob = useCancelJob()

  const [queryMode, setQueryMode] = useState<QueryMode>('blast')
  const [fasta, setFasta] = useState('')
  const [program, setProgram] = useState('blastn')
  const [database, setDatabase] = useState('')
  const [template, setTemplate] = useState('megablast')
  const [advancedParams, setAdvancedParams] = useState('')
  const [transcriptId, setTranscriptId] = useState('')
  const [selectedJobId, setSelectedJobId] = useState<string | null>(null)
  const [selectedHit, setSelectedHit] = useState<Hit | null>(null)
  const [transcriptResult, setTranscriptResult] = useState<TranscriptResult | null>(null)
  const [transcriptLoading, setTranscriptLoading] = useState(false)

  const resultsRef = useRef<HTMLDivElement>(null)
  const alignmentRef = useRef<HTMLDivElement>(null)

  const { data: jobDetail, isLoading: jobLoading, sseState } = useJobSSE(selectedJobId)

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
  }, [])

  useEffect(() => {
    if (selectedHit) {
      setTimeout(() => {
        alignmentRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' })
      }, 100)
    }
  }, [selectedHit])

  const handleSubmit = useCallback(async () => {
    const err = validateFASTA(fasta)
    if (err) {
      message.error(err)
      return
    }
    if (!database) {
      message.error('Please select a database')
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
      await createJob.mutateAsync({
        fasta: fasta.trim(),
        program,
        db: database,
        advanced_params: advParams,
      })
      message.success('Job submitted successfully')
    } catch (e: any) {
      message.error(e.message || 'Failed to submit job')
    }
  }, [fasta, program, database, template, advancedParams, createJob])

  const handleTranscriptLookup = useCallback(async (id?: string) => {
    const tid = (id || transcriptId).trim()
    if (!tid) {
      message.error('Please enter a transcript ID')
      return
    }
    if (!database) {
      message.error('Please select a database')
      return
    }
    setTranscriptLoading(true)
    try {
      const result = await lookupTranscript(database, tid)
      setTranscriptResult(result)
      if (!id) setTranscriptId(tid)
      message.success(`Found: ${result.chromosome}:${result.start}-${result.end} (${result.strand})`)
    } catch (e: any) {
      message.error(e.message || 'Transcript lookup failed')
    } finally {
      setTranscriptLoading(false)
    }
  }, [transcriptId, database])

  const handleCancel = useCallback(
    async (id: string) => {
      try {
        await cancelJob.mutateAsync(id)
        message.info('Job cancellation requested')
      } catch (e: any) {
        message.error(e.message || 'Failed to cancel job')
      }
    },
    [cancelJob],
  )

  const jobResult = jobDetail?.result as BlastResult | null
  const hits = jobResult?.results || []

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header />
      <Content style={{ padding: 24, maxWidth: 1400, margin: '0 auto', width: '100%' }}>
        {health?.status === 'degraded' && (
          <Alert
            message="System running in low-resource mode"
            description="The server has limited CPU/memory. Job throughput may be reduced."
            type="warning"
            showIcon
            style={{ marginBottom: 16 }}
          />
        )}

        <Row gutter={24}>
          {/* Left: Input Panel */}
          <Col xs={24} lg={10}>
            <Card title={<Title level={5} style={{ margin: 0 }}>Submit Job</Title>}>
              <Space direction="vertical" style={{ width: '100%' }} size="middle">
                <Radio.Group
                  value={queryMode}
                  onChange={(e) => {
                    setQueryMode(e.target.value)
                    setTranscriptResult(null)
                  }}
                  optionType="button"
                  buttonStyle="solid"
                >
                  <Radio.Button value="blast">BLAST Search</Radio.Button>
                  <Radio.Button value="transcript">Transcript Lookup</Radio.Button>
                </Radio.Group>

                {queryMode === 'transcript' && (
                  <Space direction="vertical" style={{ width: '100%' }} size="small">
                    <Form.Item label="Database" style={{ marginBottom: 0 }}>
                      <Select
                        value={database}
                        onChange={setDatabase}
                        options={databases.map((db) => ({
                          label: `${db.name} (${db.type})`,
                          value: db.name,
                        }))}
                        style={{ width: '100%' }}
                        notFoundContent="No databases configured"
                      />
                    </Form.Item>
                    <Form.Item label="Transcript ID" style={{ marginBottom: 0 }}>
                      <Input
                        value={transcriptId}
                        onChange={(e) => setTranscriptId(e.target.value)}
                        placeholder="e.g. LOC112737024"
                        onPressEnter={() => handleTranscriptLookup()}
                      />
                    </Form.Item>
                    <Button
                      type="primary"
                      onClick={() => handleTranscriptLookup()}
                      loading={transcriptLoading}
                      block
                    >
                      Lookup Sequence
                    </Button>
                  </Space>
                )}

                {queryMode === 'blast' && (
                  <>
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
                  </>
                )}
              </Space>
            </Card>
          </Col>

          {/* Right: Jobs → Results */}
          <Col xs={24} lg={14}>
            <Card
              title={<Title level={5} style={{ margin: 0 }}>Jobs</Title>}
              style={{ marginBottom: 24 }}
            >
              {jobs.length === 0 ? (
                <Text type="secondary">No jobs submitted yet</Text>
              ) : (
                <Space direction="vertical" style={{ width: '100%' }} size="small">
                  {jobs.map((job) => (
                    <JobCard
                      key={job.job_id}
                      job={job}
                      selected={selectedJobId === job.job_id}
                      onSelect={handleSelectJob}
                      onCancel={handleCancel}
                    />
                  ))}
                </Space>
              )}
            </Card>

            <div ref={resultsRef}>
              <Card
                title={
                  <Space>
                    <Title level={5} style={{ margin: 0 }}>Results</Title>
                    {selectedJobId && <Text code>{selectedJobId}</Text>}
                  </Space>
                }
                extra={
                  <Space>
                    {selectedJobId && sseState !== 'connected' && sseState !== 'disconnected' && (
                      <Tag color="warning">{sseState === 'reconnecting' ? 'Reconnecting...' : 'Connecting...'}</Tag>
                    )}
                    {selectedJobId && (
                      <Button size="small" onClick={() => { setSelectedJobId(null); setSelectedHit(null) }}>
                        Close
                      </Button>
                    )}
                  </Space>
                }
              >
                {!selectedJobId && !transcriptResult && (
                  <Text type="secondary">Select a job from the list above to view results</Text>
                )}

                {queryMode === 'transcript' && transcriptResult && (
                  <Space direction="vertical" style={{ width: '100%' }} size="middle">
                    <Alert
                      message={
                        <span>
                          {transcriptResult.chromosome}:{transcriptResult.start}-{transcriptResult.end}{' '}
                          <Tag color="blue">{transcriptResult.type}</Tag>
                          <Tag>{transcriptResult.strand === '+' ? 'Forward' : 'Reverse'}</Tag>
                        </span>
                      }
                      description={
                        transcriptResult.related ? (
                          <Space direction="vertical" size={2}>
                            <span><Text type="secondary">Gene:</Text> <Text code>{transcriptResult.gene_id}</Text></span>
                            {transcriptResult.related.transcripts.length > 0 && (
                              <span>
                                <Text type="secondary">Transcripts:</Text>{' '}
                                {transcriptResult.related.transcripts.map((t, i) => (
                                  <span key={t}>
                                    <Text code style={{ cursor: 'pointer' }}
                                      onClick={() => handleTranscriptLookup(t)}>
                                      {t}
                                    </Text>
                                    {i < transcriptResult.related!.transcripts.length - 1 && ', '}
                                  </span>
                                ))}
                              </span>
                            )}
                            {transcriptResult.related.cdss.length > 0 && (
                              <span><Text type="secondary">CDSs:</Text> {transcriptResult.related.cdss.length} regions</span>
                            )}
                          </Space>
                        ) : (
                          <Text type="secondary">Gene: {transcriptResult.gene_id}</Text>
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
                        { label: '5 kb Upstream', seq: up5, len: up5.length },
                        { label: '2 kb Upstream (proximal)', seq: up2, len: up2.length },
                        { label: `Gene Body (${geneSeq.length} bp)`, seq: geneSeq, len: geneSeq.length },
                        { label: `mRNA (exons joined, ${mrna.length} bp)`, seq: mrna, len: mrna.length },
                        { label: `CDS (joined, ${cds.length} bp)`, seq: cds, len: cds.length },
                      ].filter(r => r.len > 0)

                      return (
                        <Space direction="vertical" style={{ width: '100%' }} size="small">
                          {regionPanels.map((r) => (
                            <Card
                              key={r.label}
                              size="small"
                              title={<Text strong>{r.label} <Text type="secondary">({r.len} bp)</Text></Text>}
                              extra={
                                <Button size="small" type="link" onClick={() => {
                                  const blob = new Blob([`>${transcriptResult.transcript_id}_${r.label}\n${r.seq.match(/.{1,60}/g)?.join('\n')}`], { type: 'text/plain' })
                                  const url = URL.createObjectURL(blob)
                                  const a = document.createElement('a')
                                  a.href = url
                                  a.download = `${transcriptResult.transcript_id}_${r.label.replace(/\s+/g, '_')}.fasta`
                                  a.click()
                                  URL.revokeObjectURL(url)
                                }}>
                                  FASTA
                                </Button>
                              }
                              style={{ marginBottom: 4 }}
                            >
                              <pre
                                style={{
                                  fontFamily: '"Courier New", Courier, monospace',
                                  fontSize: 11,
                                  lineHeight: '16px',
                                  whiteSpace: 'pre-wrap',
                                  wordBreak: 'break-all',
                                  margin: 0,
                                  maxHeight: 150,
                                  overflow: 'auto',
                                  background: '#fafafa',
                                  padding: 8,
                                  borderRadius: 4,
                                }}
                              >
                                {r.seq.match(/.{1,60}/g)?.join('\n') || r.seq}
                              </pre>
                            </Card>
                          ))}
                        </Space>
                      )
                    })()}
                  </Space>
                )}

                {queryMode === 'blast' && (
                  <>
                    {jobLoading && <Spin />}

                    {jobDetail?.status === 'failed' && (
                      <Alert message="Job Failed" description={jobDetail.error} type="error" showIcon />
                    )}

                    {jobDetail?.status === 'cancelled' && (
                      <Alert message="Job Cancelled" type="warning" showIcon />
                    )}

                    {jobDetail?.status === 'running' && (
                      <Alert message={jobDetail.progress || 'Running...'} type="info" showIcon icon={<Spin />} />
                    )}

                    {jobDetail?.status === 'queued' && (
                      <Alert
                        message={`Queued (position #${jobDetail.queue_pos})`}
                        type="info"
                        showIcon
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
                              onLookupTranscript={(id) => {
                                setQueryMode('transcript')
                                setTranscriptResult(null)
                                setTranscriptId(id)
                                setTimeout(() => handleTranscriptLookup(id), 50)
                              }}
                            />
                          </div>
                        )}
                      </>
                    )}
                  </>
                )}
              </Card>
            </div>
          </Col>
        </Row>
      </Content>
    </Layout>
  )
}
