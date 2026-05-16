export default {
  async fetch(request, env) {
    const url = new URL(request.url)
    const path = url.pathname

    if (path === '/sequence' && request.method === 'GET') {
      return handleSequence(url, env)
    }

    if (path === '/health') {
      return json(200, { status: 'ok', worker: 'helixblast-gene' })
    }

    return json(404, { error: 'not found' })
  }
}

async function handleSequence(url, env) {
  const db = url.searchParams.get('db')
  const chr = url.searchParams.get('chr')
  const start = parseInt(url.searchParams.get('start'))
  const end = parseInt(url.searchParams.get('end'))
  const strand = url.searchParams.get('strand') || '+'

  if (!db || !chr || isNaN(start) || isNaN(end)) {
    return json(400, { error: 'db, chr, start, end are required' })
  }

  try {
    const sequence = await extractSequence(env, db, chr, start, end, strand)
    return json(200, {
      database: db,
      chromosome: chr,
      start: start,
      end: end,
      strand: strand,
      sequence: sequence,
    })
  } catch (err) {
    return json(500, { error: `sequence extraction failed: ${err.message}` })
  }
}

async function extractSequence(env, db, chr, start, end, strand) {
  const chrKey = `fasta/${db}/${chr}.fa.gz`
  const chrObj = await env.GENOME_BUCKET.get(chrKey)

  let region
  if (chrObj) {
    region = await extractRange(chrObj, null, start, end)
  } else {
    const genomeKey = `fasta/${db}/genome.fa.gz`
    const genomeObj = await env.GENOME_BUCKET.get(genomeKey)
    if (!genomeObj) {
      throw new Error(`chromosome ${chr} not found for ${db}`)
    }
    region = await extractRange(genomeObj, chr, start, end)
  }

  if (!region) {
    throw new Error(`range ${start}-${end} not found on ${chr} in ${db}`)
  }

  if (strand === '-') {
    return reverseComplement(region)
  }

  return region
}

async function extractRange(obj, targetChr, start, end) {
  const stream = obj.body.pipeThrough(new DecompressionStream('gzip'))
  const reader = stream.getReader()
  const decoder = new TextDecoder()

  let buffer = ''
  let inTarget = !targetChr
  let skipped = 0
  let result = ''
  const need = end - start + 1

  // Determine line length from first sequence lines
  let firstLineLen = 0
  let probed = false
  let longLineMode = false

  while (true) {
    const { done, value } = await reader.read()
    if (done) {
      // Process remaining buffer
      if (buffer) {
        // Process trailing data (no complete line, or last chunk of long line)
        const lines = buffer.split('\n')
        for (const line of lines) {
          const trimmed = line.trim()
          if (!trimmed) continue
          if (trimmed.startsWith('>')) break
          if (!inTarget) continue
          if (skipped + trimmed.length < start) { skipped += trimmed.length; continue }
          const offset = Math.max(0, start - skipped - 1)
          const remaining = need - result.length
          result += trimmed.slice(offset, offset + remaining)
          skipped += trimmed.length
        }
        return result || null
      }
      break
    }
    buffer += decoder.decode(value, { stream: true })

    if (!probed) {
      // Probe first chunk for line length
      const lines = buffer.split('\n')
      for (const line of lines) {
        const t = line.trim()
        if (!t.startsWith('>') && t.length > 0) {
          firstLineLen = Math.max(firstLineLen, t.length)
        }
      }
      if (firstLineLen > 0) {
        probed = true
        longLineMode = firstLineLen > 120
      }
    }

    if (longLineMode) {
      // Long-line: process char by char, clear buffer when done
      for (let i = 0; i < buffer.length; i++) {
        const ch = buffer[i]
        if (ch === '\n') continue
        if (ch === '\r') continue

        if (ch === '>') {
          if (result.length >= need) return result
          if (targetChr) {
            let headerEnd = buffer.indexOf('\n', i)
            if (headerEnd === -1) headerEnd = buffer.length
            const headerName = buffer.slice(i + 1, headerEnd).split(/\s/)[0]
            inTarget = headerName === targetChr
            if (inTarget) skipped = 0
            i = headerEnd
          }
          continue
        }

        if (/\s/.test(ch)) continue
        if (!inTarget) continue

        if (skipped + 1 < start) {
          skipped++
          continue
        }

        result += ch
        skipped++
        if (result.length >= need) return result
      }
      buffer = ''  // clear to free memory
    } else {
      // Short-line: process complete lines, keep only incomplete tail
      let nl = buffer.lastIndexOf('\n')
      if (nl === -1) continue  // no complete line yet

      // Process lines without copying the entire buffer
      let pos = 0
      while (pos <= nl) {
        const nextNl = buffer.indexOf('\n', pos)
        if (nextNl === -1) break
        const line = buffer.slice(pos, nextNl).trim()
        pos = nextNl + 1

        if (!line) continue

        if (line.startsWith('>')) {
          if (result.length >= need) return result
          if (targetChr) {
            const headerName = line.slice(1).split(/\s/)[0]
            inTarget = headerName === targetChr
            if (inTarget) skipped = 0
          }
          continue
        }

        if (!inTarget) continue

        if (skipped + line.length < start) {
          skipped += line.length
          continue
        }

        const offset = Math.max(0, start - skipped - 1)
        const remaining = need - result.length
        result += line.slice(offset, offset + remaining)
        skipped += line.length

        if (result.length >= need) return result
      }

      // Keep only the incomplete line after last newline
      buffer = buffer.slice(nl + 1)
    }
  }

  return result || null
}

function reverseComplement(seq) {
  const comp = {
    'A': 'T', 'T': 'A', 'C': 'G', 'G': 'C',
    'a': 't', 't': 'a', 'c': 'g', 'g': 'c',
    'N': 'N', 'n': 'n',
    'R': 'Y', 'Y': 'R', 'S': 'S', 'W': 'W',
    'K': 'M', 'M': 'K', 'B': 'V', 'V': 'B',
    'D': 'H', 'H': 'D',
  }
  return seq.split('').reverse().map(c => comp[c] || c).join('')
}

function json(status, data) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { 'Content-Type': 'application/json', 'Access-Control-Allow-Origin': '*' },
  })
}
