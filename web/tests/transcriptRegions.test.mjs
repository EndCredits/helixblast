import { test } from 'node:test'
import assert from 'node:assert/strict'
import { sliceTranscriptRegions } from '../.test-dist/lib/transcriptRegions.js'

function result(overrides) {
  return {
    transcript_id: 'tx',
    database: 'db',
    chromosome: 'Chr1',
    start: 10,
    end: 20,
    strand: '+',
    type: 'mRNA',
    gene_id: 'g1',
    sequence: 'AAAAAabcdefghijk',
    scan_start: 5,
    scan_end: 20,
    regions: {
      exons: [{ start: 10, end: 12 }, { start: 15, end: 20 }],
      cdss: [{ start: 10, end: 12 }],
    },
    ...overrides,
  }
}

test('plus strand keeps upstream first and stitches exons ascending', () => {
  const regions = sliceTranscriptRegions(result())
  assert.equal(regions.upstream5kb, 'AAAAA')
  assert.equal(regions.upstream2kb, 'AAAAA')
  assert.equal(regions.geneBody, 'abcdefghijk')
  assert.equal(regions.mrna, 'abcfghijk')
  assert.equal(regions.cds, 'abc')
})

test('minus strand puts upstream first, keeps gene body reverse-complemented, and stitches exons descending', () => {
  const regions = sliceTranscriptRegions(result({
    strand: '-',
    scan_start: 10,
    scan_end: 25,
  }))
  assert.equal(regions.upstream5kb, 'AAAAA')
  assert.equal(regions.geneBody, 'abcdefghijk')
  assert.equal(regions.mrna, 'abcdefijk')
  assert.equal(regions.cds, 'ijk')
})

test('2 kb upstream is the proximal end of the 5 kb flank on both strands', () => {
  for (const strand of ['+', '-']) {
    const upstreamLength = strand === '+' ? 3000 : 5000
    const regions = sliceTranscriptRegions(result({
      strand,
      start: 3001,
      end: 3100,
      scan_start: strand === '+' ? 1 : 3001,
      scan_end: strand === '+' ? 3100 : 8100,
      sequence: 'A'.repeat(upstreamLength) + 'G'.repeat(100),
      regions: undefined,
    }))
    assert.equal(regions.upstream5kb.length, upstreamLength)
    assert.equal(regions.upstream2kb, 'A'.repeat(2000))
    assert.equal(regions.geneBody, 'G'.repeat(100))
    assert.equal(regions.mrna, '')
    assert.equal(regions.cds, '')
  }
})

test('missing regions produce empty mRNA and CDS without throwing', () => {
  const regions = sliceTranscriptRegions(result({ regions: undefined }))
  assert.equal(regions.mrna, '')
  assert.equal(regions.cds, '')
})
