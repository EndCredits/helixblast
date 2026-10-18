export interface PositionSpan {
  end: number
  next: number
  label: string
}

// advancePosition walks one wrapped alignment chunk. BLAST reports a
// descending start-end pair for a minus-strand alignment, so the coordinate
// must step down rather than up.
export function advancePosition(start: number, direction: 1 | -1, chunk: string): PositionSpan {
  const bases = chunk.replace(/-/g, '').length
  const end = bases > 0 ? start + direction * (bases - 1) : start
  return {
    end,
    next: end + direction,
    label: `${start}-${end}`,
  }
}
