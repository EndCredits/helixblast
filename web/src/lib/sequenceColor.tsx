import type { ReactNode } from 'react'
import { useNucleotideColors } from '../themeMode'

const NUCLEOTIDE_ALPHABET = /^[ACGTUNRYKMSWBHDV.\-*\s]+$/i
const MAX_COLORED_LENGTH = 5000

export function isNucleotideSequence(seq: string): boolean {
  const content = seq.replace(/\s/g, '')
  return (
    content.length > 0 &&
    content.length <= MAX_COLORED_LENGTH &&
    NUCLEOTIDE_ALPHABET.test(content)
  )
}

export default function SequenceText({ seq }: { seq: string }) {
  const { colors, gap, ambiguous } = useNucleotideColors()

  if (!isNucleotideSequence(seq)) return <>{seq}</>

  const colorForChar = (ch: string): string | null => {
    if (ch === '-' || ch === '.') return gap
    if (/\s/.test(ch)) return null
    const upper = ch.toUpperCase()
    if (upper in colors) return colors[upper]
    return ambiguous
  }

  const nodes: ReactNode[] = []
  let run = ''
  let runColor: string | null = null

  const flush = () => {
    if (!run) return
    nodes.push(
      runColor ? (
        <span key={nodes.length} style={{ color: runColor }}>
          {run}
        </span>
      ) : (
        run
      ),
    )
    run = ''
  }

  for (const ch of seq) {
    const color = colorForChar(ch)
    if (color !== runColor) {
      flush()
      runColor = color
    }
    run += ch
  }
  flush()

  return <>{nodes}</>
}
