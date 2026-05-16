import { Form, Select, Collapse, Input, Button, Space } from 'antd'
import type { Database } from '../api/client'

interface Props {
  databases: Database[]
  program: string
  database: string
  template: string
  advancedParams: string
  onProgramChange: (v: string) => void
  onDatabaseChange: (v: string) => void
  onTemplateChange: (v: string) => void
  onAdvancedParamsChange: (v: string) => void
  onSubmit: () => void
  loading: boolean
}

const taskTemplates: Record<string, { label: string; desc: string }[]> = {
  blastn: [
    { label: 'megablast', desc: 'Highly similar sequences' },
    { label: 'dc-megablast', desc: 'Discontiguous megablast' },
    { label: 'blastn', desc: 'Somewhat similar sequences' },
    { label: 'blastn-short', desc: 'Short sequences (<50 bp)' },
  ],
  blastp: [
    { label: 'blastp', desc: 'Standard protein BLAST' },
    { label: 'blastp-short', desc: 'Short peptides (<30 aa)' },
    { label: 'blastp-fast', desc: 'Faster protein search' },
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
  { label: 'blastn (Nucleotide)', value: 'blastn' },
  { label: 'blastp (Protein)', value: 'blastp' },
  { label: 'blastx (Translated)', value: 'blastx' },
  { label: 'tblastn (Translated)', value: 'tblastn' },
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

  const handleProgramChange = (v: string) => {
    onProgramChange(v)
    onTemplateChange('')
  }

  return (
    <Form layout="vertical">
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        <Space wrap>
          <Form.Item label="Program" style={{ marginBottom: 0 }}>
            <Select
              value={program}
              onChange={handleProgramChange}
              options={programs}
              style={{ width: 200 }}
            />
          </Form.Item>
          <Form.Item label="Database" style={{ marginBottom: 0 }}>
            <Select
              value={database}
              onChange={onDatabaseChange}
              options={databases.map((db) => ({
                label: `${db.name} (${db.type})`,
                value: db.name,
              }))}
              style={{ width: 240 }}
              notFoundContent="No databases configured"
            />
          </Form.Item>
        </Space>

        <Collapse
          ghost
          items={[
            {
              key: 'template',
              label: 'Task Presets',
              children: (
                <Select
                  value={template}
                  onChange={onTemplateChange}
                  allowClear
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
              ),
            },
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
