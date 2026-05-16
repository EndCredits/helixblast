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

const templates: Record<string, string> = {
  megablast: '-task megablast',
  'blastn-short': '-task blastn-short -word_size 7 -evalue 1000',
  'dc-megablast': '-task dc-megablast',
  'blastp-short': '-task blastp-short -word_size 2 -evalue 20000',
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
  const templateOptions = Object.entries(templates).map(([k, v]) => ({
    label: `${k}`,
    value: k,
    desc: v,
  }))

  return (
    <Form layout="vertical">
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        <Space wrap>
          <Form.Item label="Program" style={{ marginBottom: 0 }}>
            <Select
              value={program}
              onChange={onProgramChange}
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
              label: 'Quick Templates',
              children: (
                <Select
                  value={template}
                  onChange={onTemplateChange}
                  allowClear
                  placeholder="Select a preset template..."
                  options={templateOptions.map((t) => ({
                    label: `${t.label}  —  ${t.desc}`,
                    value: t.value,
                  }))}
                  style={{ width: '100%' }}
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
                  placeholder="-word_size 11 -evalue 1e-5"
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
