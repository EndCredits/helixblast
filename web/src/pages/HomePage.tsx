import { useState, useCallback, useMemo, useRef, useEffect } from 'react'
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
import type { Hit, BlastResult } from '../api/client'

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

export default function HomePage() {
  const { data: health } = useHealth()
  const { data: databases = [] } = useDatabases()
  const { data: jobs = [] } = useJobs()
  const createJob = useCreateJob()
  const cancelJob = useCancelJob()

  const [fasta, setFasta] = useState('')
  const [program, setProgram] = useState('blastn')
  const [database, setDatabase] = useState('')
  const [template, setTemplate] = useState('megablast')
  const [advancedParams, setAdvancedParams] = useState('')
  const [selectedJobId, setSelectedJobId] = useState<string | null>(null)
  const [selectedHit, setSelectedHit] = useState<Hit | null>(null)

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
                {!selectedJobId && (
                  <Text type="secondary">Select a job from the list above to view results</Text>
                )}

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
                        <AlignmentView hit={selectedHit} />
                      </div>
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
