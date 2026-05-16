import { useMemo } from 'react'
import { Typography, Space, Empty } from 'antd'
import type { Hit } from '../api/client'

const { Text } = Typography

interface Props {
  hit: Hit | null
}

function buildAlignment(hit: Hit): { query: string; match: string; subject: string }[] {
  const blocks: { query: string; match: string; subject: string }[] = []

  for (const hsp of hit.alignments) {
    let matchLine = ''
    const q = hsp.query_seq
    const s = hsp.subject_seq

    for (let i = 0; i < q.length; i++) {
      if (i < s.length && q[i].toUpperCase() === s[i].toUpperCase()) {
        matchLine += '|'
      } else if (i < s.length && q[i] !== '-' && s[i] !== '-') {
        matchLine += ' '
      } else {
        matchLine += ' '
      }
    }

    blocks.push({
      query: q,
      match: matchLine,
      subject: s,
    })
  }

  return blocks
}

export default function AlignmentView({ hit }: Props) {
  const blocks = useMemo(() => {
    if (!hit) return []
    return buildAlignment(hit)
  }, [hit])

  if (!hit) {
    return <Empty description="Select a hit to view alignment" />
  }

  return (
    <Space direction="vertical" style={{ width: '100%' }} size="small">
      <Text strong>{hit.subject_id}</Text>
      <div
        style={{
          background: '#fafafa',
          borderRadius: 6,
          padding: 12,
          overflowX: 'auto',
          maxHeight: 400,
          overflowY: 'auto',
        }}
      >
        {blocks.map((block, idx) => (
          <pre
            key={idx}
            style={{
              fontFamily: '"Courier New", Courier, monospace',
              fontSize: 12,
              lineHeight: '16px',
              margin: '0 0 8px 0',
              whiteSpace: 'pre',
            }}
          >
            <div style={{ color: '#1677ff' }}>
              Query   {Math.max(1, hit.alignments[idx]?.query_start ?? 0)}{' '}
              {block.query}{' '}
              {hit.alignments[idx]?.query_end ?? 0}
            </div>
            <div style={{ color: '#52c41a' }}>
              {'       '}
              {' '.repeat(String(hit.alignments[idx]?.query_start ?? 0).length)}{' '}
              {block.match}
            </div>
            <div style={{ color: '#fa541c' }}>
              Sbjct   {hit.alignments[idx]?.subject_start ?? 0}{' '}
              {block.subject}{' '}
              {hit.alignments[idx]?.subject_end ?? 0}
            </div>
          </pre>
        ))}
      </div>
    </Space>
  )
}
