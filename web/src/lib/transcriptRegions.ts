import type { TranscriptResult } from '../api/client'

export interface TranscriptRegionSequences {
  upstream5kb: string
  upstream2kb: string
  geneBody: string
  mrna: string
  cds: string
}

interface RegionCoord {
  start: number
  end: number
}

function transcriptOrder(regions: RegionCoord[], strand: string): RegionCoord[] {
  return [...regions].sort((a, b) =>
    strand === '-' ? b.start - a.start : a.start - b.start,
  )
}

export function sliceTranscriptRegions(result: TranscriptResult): TranscriptRegionSequences {
  const minus = result.strand === '-'
  const upstreamLength = Math.max(
    0,
    minus ? result.scan_end - result.end : result.start - result.scan_start,
  )
  const geneLength = Math.max(0, result.end - result.start + 1)
  const upstream5kb = result.sequence.slice(0, upstreamLength)
  const upstream2kb = upstream5kb.slice(Math.max(0, upstreamLength - 2000))
  const geneBody = result.sequence.slice(upstreamLength, upstreamLength + geneLength)

  const extract = (region: RegionCoord): string => {
    const offset = minus ? result.end - region.end : region.start - result.start
    const length = region.end - region.start + 1
    if (offset < 0 || length <= 0) return ''
    return geneBody.slice(offset, offset + length)
  }

  const mrna = transcriptOrder(result.regions?.exons ?? [], result.strand)
    .map(extract)
    .join('')
  const cds = transcriptOrder(result.regions?.cdss ?? [], result.strand)
    .map(extract)
    .join('')

  return { upstream5kb, upstream2kb, geneBody, mrna, cds }
}
