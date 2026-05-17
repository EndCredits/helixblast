import { Form, Select, Collapse, Input, Button, Space, Tag } from 'antd'
import type { Database } from '../api/client'

interface Props {
  databases: Database[]
  program: string
  database: string[]
  template: string
  advancedParams: string
  onProgramChange: (v: string) => void
  onDatabaseChange: (v: string[]) => void
  onTemplateChange: (v: string) => void
  onAdvancedParamsChange: (v: string) => void
  onSubmit: () => void
  loading: boolean
}

const taskTemplates: Record<string, { label: string; desc: string }[]> = {
  blastn: [
    { label: 'megablast', desc: 'Highly similar sequences' },
    { label: 'blastn', desc: 'Somewhat similar sequences' },
    { label: 'dc-megablast', desc: 'Discontiguous megablast' },
    { label: 'blastn-short', desc: 'Short sequences (<50 bp)' },
  ],
  blastp: [
    { label: 'blastp', desc: 'Standard protein BLAST' },
    { label: 'blastp-fast', desc: 'Faster protein search' },
    { label: 'blastp-short', desc: 'Short peptides (<30 aa)' },
  ],
  blastx: [
    { label: 'blastx', desc: 'Standard translated search' },
    { label: 'blastx-fast', desc: 'Faster translated search' },
  ],
  tblastn: [
    { label: 'tblastn', desc: 'Standard translated search' },
    { label: 'tblastn-fast', desc: 'Faster translated search' },
  ],
}

const programs = [
  { label: 'blastn (Nucleotide → Nucleotide)', value: 'blastn' },
  { label: 'blastp (Protein → Protein)', value: 'blastp' },
  { label: 'blastx (Translated DNA → Protein)', value: 'blastx' },
  { label: 'tblastn (Protein → Translated DNA)', value: 'tblastn' },
]

export default function ParamPanel({
  databases,
  program,
  database,
  template,
  advancedParams,
  onProgramChange,
  onDatabaseChange,
  onTemplateChange,
  onAdvancedParamsChange,
  onSubmit,
  loading,
}: Props) {
  const currentTasks = taskTemplates[program] || taskTemplates.blastn
  const defaultTask = currentTasks[0]?.label || ''

  const handleProgramChange = (v: string) => {
    onProgramChange(v)
    const tasks = taskTemplates[v] || taskTemplates.blastn
    onTemplateChange(tasks[0]?.label || '')
  }

  return (
    <Form layout="vertical">
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        <Form.Item label="Program" style={{ marginBottom: 0 }}>
          <Select
            value={program}
            onChange={handleProgramChange}
            options={programs}
            style={{ width: '100%' }}
          />
        </Form.Item>

        <Form.Item label="Database" style={{ marginBottom: 0 }}>
          {database.length > 0 && (
            <Space wrap style={{ marginBottom: 8 }}>
              {database.map((db) => (
                <Tag
                  key={db}
                  closable
                  color="blue"
                  onClose={() => onDatabaseChange(database.filter((d) => d !== db))}
                >
                  {db}
                </Tag>
              ))}
            </Space>
          )}
          <Select
            mode="multiple"
            value={database}
            onChange={onDatabaseChange}
            options={databases.map((db) => ({
              label: `${db.name} (${db.type})`,
              value: db.name,
            }))}
            style={{ width: '100%' }}
            placeholder="Select one or more databases..."
            notFoundContent="No databases configured"
          />
        </Form.Item>

        <Form.Item label="Task Preset" style={{ marginBottom: 0 }}>
          <Select
            value={template || defaultTask}
            onChange={onTemplateChange}
            placeholder="Select a task preset..."
            options={currentTasks.map((t) => ({
              label: t.label,
              value: t.label,
              desc: t.desc,
            }))}
            style={{ width: '100%' }}
            optionRender={(option) => (
              <Space>
                <span style={{ fontFamily: 'monospace', fontWeight: 600 }}>
                  {option.data.label}
                </span>
                <span style={{ color: '#888', fontSize: 12 }}>
                  {option.data.desc}
                </span>
              </Space>
            )}
          />
        </Form.Item>

        <Collapse
          ghost
          items={[
            {
              key: 'advanced',
              label: 'Advanced Parameters',
              children: (
                <Input.TextArea
                  rows={3}
                  value={advancedParams}
                  onChange={(e) => onAdvancedParamsChange(e.target.value)}
                  placeholder="-word_size 11 -evalue 1e-5 -max_target_seqs 500"
                  style={{ fontFamily: 'monospace', fontSize: 13 }}
                />
              ),
            },
          ]}
        />

        <Button
          type="primary"
          size="large"
          onClick={onSubmit}
          loading={loading}
          block
        >
          Submit BLAST Job
        </Button>
      </Space>
    </Form>
  )
}
