import { useMemo, useCallback } from 'react'
import { Typography, Space, Empty, Button, Divider } from 'antd'
import { DownloadOutlined, SearchOutlined, EnvironmentOutlined } from '@ant-design/icons'
import type { Hit } from '../api/client'

const { Text } = Typography

const LINE_LEN = 60

interface Props {
  hit: Hit | null
  onLookupTranscript?: (id: string) => void
  onLookupRegion?: (chr: string, pos: number) => void
}

interface AlignmentBlock {
  queryStart: number
  queryEnd: number
  subjectStart: number
  subjectEnd: number
  queryLines: string[]
  matchLines: string[]
  subjectLines: string[]
  queryPositions: string[]
  subjectPositions: string[]
}

function buildWrappedAlignment(hit: Hit): AlignmentBlock[] {
  const blocks: AlignmentBlock[] = []

  for (const hsp of hit.alignments) {
    const q = hsp.query_seq
    const s = hsp.subject_seq
    const queryLines: string[] = []
    const matchLines: string[] = []
    const subjectLines: string[] = []
    const queryPositions: string[] = []
    const subjectPositions: string[] = []

    let qPos = hsp.query_start
    let sPos = hsp.subject_start

    for (let offset = 0; offset < q.length; offset += LINE_LEN) {
      const qChunk = q.slice(offset, offset + LINE_LEN)
      const sChunk = s.slice(offset, offset + LINE_LEN)

      let match = ''
      for (let i = 0; i < qChunk.length; i++) {
        const qi = qChunk[i].toUpperCase()
        const si = i < sChunk.length ? sChunk[i].toUpperCase() : ''
        if (qi === si && qi !== '-') {
          match += '|'
        } else if (qi !== '-' && si !== '-' && qi !== si) {
          match += ' '
        } else {
          match += ' '
        }
      }

      const qEnd = qPos + qChunk.replace(/-/g, '').length - 1
      const sEnd = sPos + sChunk.replace(/-/g, '').length - 1

      queryLines.push(qChunk)
      matchLines.push(match)
      subjectLines.push(sChunk)
      queryPositions.push(`${qPos}-${qEnd}`)
      subjectPositions.push(`${sPos}-${sEnd}`)

      qPos = qEnd + 1
      sPos = sEnd + 1
    }

    blocks.push({
      queryStart: hsp.query_start,
      queryEnd: hsp.query_end,
      subjectStart: hsp.subject_start,
      subjectEnd: hsp.subject_end,
      queryLines,
      matchLines,
      subjectLines,
      queryPositions,
      subjectPositions,
    })
  }

  return blocks
}

function buildFASTA(hit: Hit): string {
  const lines: string[] = []
  const qid = 'Query'
  const sid = hit.subject_id

  for (const hsp of hit.alignments) {
    lines.push(`>${qid} ${hsp.query_start}-${hsp.query_end}`)
    const q = hsp.query_seq
    for (let i = 0; i < q.length; i += LINE_LEN) {
      lines.push(q.slice(i, i + LINE_LEN))
    }
    lines.push(`>${sid} ${hsp.subject_start}-${hsp.subject_end}`)
    const s = hsp.subject_seq
    for (let i = 0; i < s.length; i += LINE_LEN) {
      lines.push(s.slice(i, i + LINE_LEN))
    }
  }

  return lines.join('\n')
}

export default function AlignmentView({ hit, onLookupTranscript, onLookupRegion }: Props) {
  const blocks = useMemo(() => {
    if (!hit) return []
    return buildWrappedAlignment(hit)
  }, [hit])

  const handleExportFASTA = useCallback(() => {
    if (!hit) return
    const content = buildFASTA(hit)
    const blob = new Blob([content], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${hit.subject_id}_alignment.fasta`
    a.click()
    URL.revokeObjectURL(url)
  }, [hit])

  if (!hit) {
    return <Empty description="Select a hit to view alignment" />
  }

  return (
    <Space direction="vertical" style={{ width: '100%' }} size="small">
      <Space style={{ justifyContent: 'space-between', width: '100%' }}>
        <Text strong>
          {hit.subject_id} — {hit.identity.toFixed(1)}% identity, E-value: {hit.e_value}
        </Text>
        <Space>
          {onLookupTranscript && (
            <Button size="small" icon={<SearchOutlined />} onClick={() => onLookupTranscript(hit.subject_id)}>
              Query Transcript
            </Button>
          )}
          {onLookupRegion && hit.alignments.length > 0 && (
            <Button size="small" icon={<EnvironmentOutlined />} onClick={() => {
              const pos = Math.floor((hit.alignments[0].subject_start + hit.alignments[0].subject_end) / 2)
              onLookupRegion(hit.subject_id, pos)
            }}>
              Lookup Region
            </Button>
          )}
          <Button size="small" icon={<DownloadOutlined />} onClick={handleExportFASTA}>
            Export FASTA
          </Button>
        </Space>
      </Space>
      <div
        style={{
          background: '#fafafa',
          borderRadius: 6,
          padding: 12,
          overflowX: 'auto',
          maxHeight: 400,
          overflowY: 'auto',
          fontFamily: '"Courier New", Courier, monospace',
          fontSize: 12,
          lineHeight: '16px',
        }}
      >
        {blocks.map((block, idx) => (
          <div key={idx} style={{ marginBottom: 12 }}>
            {blocks.length > 1 && (
              <div style={{ fontSize: 11, color: '#888', marginBottom: 4 }}>
                HSP {idx + 1}/{blocks.length}
                {idx < blocks.length - 1 && <Divider style={{ margin: '4px 0' }} />}
              </div>
            )}
            {block.queryLines.map((qLine, lineIdx) => (
              <pre
                key={lineIdx}
                style={{
                  margin: 0,
                  fontFamily: 'inherit',
                  fontSize: 'inherit',
                  lineHeight: 'inherit',
                  whiteSpace: 'pre',
                }}
              >
                {(() => {
                  const qPos = block.queryPositions[lineIdx]
                  const sPos = block.subjectPositions[lineIdx]
                  const maxPosLen = Math.max(qPos.length, sPos.length)
                  const labelWidth = 6
                  const prefix = ' '.repeat(labelWidth)
                  return (
                    <>
                      <span style={{ color: '#1677ff' }}>
                        {'Query '}{qPos.padStart(maxPosLen)}{' '}{qLine}
                      </span>
                      {'\n'}
                      <span style={{ color: '#52c41a' }}>
                        {prefix}{' '.repeat(maxPosLen)}{' '}{block.matchLines[lineIdx]}
                      </span>
                      {'\n'}
                      <span style={{ color: '#fa541c' }}>
                        {'Sbjct '}{sPos.padStart(maxPosLen)}{' '}{block.subjectLines[lineIdx]}
                      </span>
                    </>
                  )
                })()}
              </pre>
            ))}
            {idx < blocks.length - 1 && <div style={{ height: 8 }} />}
          </div>
        ))}
      </div>
    </Space>
  )
}
