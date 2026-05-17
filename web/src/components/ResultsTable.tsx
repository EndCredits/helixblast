import { Table, Tag, Typography, Empty } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { Hit } from '../api/client'

const { Text } = Typography

interface Props {
  hits: Hit[]
  onSelectHit: (hit: Hit) => void
}

export default function ResultsTable({ hits, onSelectHit }: Props) {
  if (!hits || hits.length === 0) {
    return <Empty description="No results to display" />
  }

  const columns: ColumnsType<Hit> = [
    {
      title: '#',
      key: 'index',
      width: 50,
      render: (_, __, i) => i + 1,
    },
    {
      title: 'Subject ID',
      dataIndex: 'subject_id',
      key: 'subject_id',
      render: (id: string) => <Text code>{id}</Text>,
      sorter: (a, b) => a.subject_id.localeCompare(b.subject_id),
    },
    {
      title: 'Database',
      dataIndex: 'database',
      key: 'database',
      width: 110,
      render: (db: string) => db ? <Tag color="blue">{db}</Tag> : null,
    },
    {
      title: 'Identity',
      dataIndex: 'identity',
      key: 'identity',
      width: 100,
      render: (v: number) => (
        <Tag color={v >= 98 ? 'success' : v >= 80 ? 'processing' : 'default'}>
          {v.toFixed(1)}%
        </Tag>
      ),
      sorter: (a, b) => a.identity - b.identity,
      defaultSortOrder: 'descend',
    },
    {
      title: 'Coverage',
      dataIndex: 'coverage',
      key: 'coverage',
      width: 100,
      render: (v: number) => `${v.toFixed(1)}%`,
      sorter: (a, b) => a.coverage - b.coverage,
    },
    {
      title: 'E-value',
      dataIndex: 'e_value',
      key: 'e_value',
      width: 120,
      sorter: (a, b) => {
        const ea = parseFloat(a.e_value) || 0
        const eb = parseFloat(b.e_value) || 0
        return ea - eb
      },
    },
    {
      title: 'Bitscore',
      dataIndex: 'total_score',
      key: 'total_score',
      width: 100,
      render: (v: number) => v.toFixed(1),
      sorter: (a, b) => a.total_score - b.total_score,
    },
    {
      title: 'Alignments',
      key: 'alignments',
      width: 100,
      render: (_, record) => `${record.alignments?.length || 0} HSPs`,
    },
  ]

  return (
    <Table<Hit>
      columns={columns}
      dataSource={hits}
      rowKey={(record) => `${record.database || 'unknown'}:${record.subject_id}`}
      size="small"
      sticky
      pagination={false}
      onRow={(record) => ({
        onClick: () => onSelectHit(record),
        style: { cursor: 'pointer' },
      })}
      scroll={{ x: 'max-content' }}
    />
  )
}
