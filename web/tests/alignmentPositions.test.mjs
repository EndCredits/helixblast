import { test } from 'node:test'
import assert from 'node:assert/strict'
import { advancePosition } from '../.test-dist/lib/alignmentPositions.js'

test('plus-strand positions step upward', () => {
  const span = advancePosition(10, 1, 'ACGT--AC')
  assert.equal(span.end, 15)
  assert.equal(span.next, 16)
  assert.equal(span.label, '10-15')
})

test('minus-strand positions step downward', () => {
  const span = advancePosition(20, -1, 'ACGT--AC')
  assert.equal(span.end, 15)
  assert.equal(span.next, 14)
  assert.equal(span.label, '20-15')
})

test('a gap-only chunk does not move the coordinate', () => {
  const span = advancePosition(10, -1, '---')
  assert.equal(span.end, 10)
  assert.equal(span.next, 9)
  assert.equal(span.label, '10-10')
})
