const fs = require('fs')
const zlib = require('zlib')
const readline = require('readline')

async function parseGFF3(filePath) {
  const raw = []
  const parentMap = {}
  const coordMap = {}
  const nameAlias = {}

  const fileStream = fs.createReadStream(filePath)
  const rl = readline.createInterface({ input: fileStream })

  for await (const line of rl) {
    if (line.startsWith('#') || line.trim() === '') continue

    const cols = line.split('\t')
    if (cols.length < 9) continue

    const [chr, , type, start, end, , strand, , attrs] = cols
    const id = extractAttr(attrs, 'ID')
    const parent = extractAttr(attrs, 'Parent')
    const name = extractAttr(attrs, 'Name')

    if (!id) continue

    raw.push({ id, parent, type, chr, start: parseInt(start), end: parseInt(end), strand, name })

    if (parent) {
      parentMap[id] = parent
    }
    if (name) {
      nameAlias[name] = id
    }

    if (type === 'gene' || type === 'mRNA' || type === 'transcript') {
      coordMap[id] = { chr, start: parseInt(start), end: parseInt(end), strand }
    }
  }

  const resolveCoords = (id) => {
    if (coordMap[id]) return coordMap[id]
    const parent = parentMap[id]
    if (!parent) return null
    return resolveCoords(parent)
  }

  const resolveGene = (id) => {
    const parent = parentMap[id]
    if (!parent) return id
    let cur = parent
    while (parentMap[cur]) {
      cur = parentMap[cur]
    }
    return cur
  }

  const index = {}
  const families = {}
  const coords = {}

  for (const entry of raw) {
    const coordsResolved = resolveCoords(entry.id)
    if (!coordsResolved) continue

    const gene = resolveGene(entry.id)

    index[entry.id] = {
      chr: coordsResolved.chr,
      start: coordsResolved.start,
      end: coordsResolved.end,
      strand: coordsResolved.strand,
      type: entry.type,
      gene: gene,
    }

    if (entry.name && entry.name !== entry.id) {
      index[entry.name] = index[entry.id]
    }

    if (!families[gene]) {
      families[gene] = { transcripts: [], cdss: [], exons: [] }
    }

    switch (entry.type) {
      case 'mRNA':
      case 'transcript':
        if (!families[gene].transcripts.includes(entry.id)) {
          families[gene].transcripts.push(entry.id)
        }
        if (!coords[entry.id]) {
          coords[entry.id] = { exons: [], cdss: [] }
        }
        break
      case 'CDS':
        if (!families[gene].cdss.includes(entry.id)) {
          families[gene].cdss.push(entry.id)
        }
        if (entry.parent) {
          const mrna = entry.parent
          if (!coords[mrna]) coords[mrna] = { exons: [], cdss: [] }
          coords[mrna].cdss.push({ start: entry.start, end: entry.end })
        }
        break
      case 'exon':
        if (!families[gene].exons.includes(entry.id)) {
          families[gene].exons.push(entry.id)
        }
        if (entry.parent) {
          const mrna = entry.parent
          if (!coords[mrna]) coords[mrna] = { exons: [], cdss: [] }
          coords[mrna].exons.push({ start: entry.start, end: entry.end })
        }
        break
    }
  }

  for (const id of Object.keys(coords)) {
    coords[id].exons.sort((a, b) => a.start - b.start)
    coords[id].cdss.sort((a, b) => a.start - b.start)
  }

  return { index, families, coords }
}

function extractAttr(attrs, key) {
  const parts = attrs.split(';')
  for (const p of parts) {
    const eq = p.trim().indexOf('=')
    if (eq === -1) continue
    const k = p.slice(0, eq).trim()
    const v = p.slice(eq + 1).trim()
    if (k === key) return v
  }
  return null
}

async function buildFastaIndex(fastaFile) {
  const index = {}
  const fileStream = fs.createReadStream(fastaFile, { encoding: 'utf8' })
  let offset = 0
  let leftover = ''

  for await (const chunk of fileStream) {
    const text = leftover + chunk
    const lines = text.split('\n')
    leftover = lines.pop()

    for (const line of lines) {
      const trimmed = line.trim()
      if (trimmed.startsWith('>')) {
        const header = trimmed.slice(1).split(/\s/)[0]
        index[header] = offset
      }
      offset += Buffer.byteLength(line, 'utf8') + 1
    }
  }

  return index
}

async function main() {
  const args = process.argv.slice(2)
  if (args.length < 3) {
    console.error('Usage: node prepare.js <gff3_file> <output_prefix> <fasta_file>')
    process.exit(1)
  }

  const inputFile = args[0]
  const outputPrefix = args[1]
  const fastaFile = args[2]

  console.log(`Parsing ${inputFile}...`)
  const { index, families, coords } = await parseGFF3(inputFile)

  const ids = Object.keys(index)
  const types = {}
  for (const id of ids) {
    const t = index[id].type
    types[t] = (types[t] || 0) + 1
  }

  console.log(`Indexed ${ids.length} unique IDs:`)
  for (const [t, n] of Object.entries(types).sort()) {
    console.log(`  ${t}: ${n}`)
  }
  console.log(`Gene families: ${Object.keys(families).length}`)

  console.log(`\nIndexing FASTA: ${fastaFile}`)
  const fastaIndex = await buildFastaIndex(fastaFile)
  console.log(`Indexed ${Object.keys(fastaIndex).length} chromosomes`)

  const output = { entries: index, families: families, coords: coords, fasta_index: fastaIndex }
  const jsonStr = JSON.stringify(output)

  const gzPath = `${outputPrefix}.index.json.gz`
  const gzStream = zlib.createGzip()
  const gzOut = fs.createWriteStream(gzPath)
  await new Promise((resolve, reject) => {
    const buf = Buffer.from(jsonStr)
    gzOut.on('finish', resolve)
    gzOut.on('error', reject)
    gzStream.pipe(gzOut)
    gzStream.end(buf)
  })
  console.log(`\nWritten: ${gzPath} (${(fs.statSync(gzPath).size / 1024).toFixed(1)} KB)`)

  const jsonPath = `${outputPrefix}.index.json`
  fs.writeFileSync(jsonPath, jsonStr)
  console.log(`Written: ${jsonPath} (${(fs.statSync(jsonPath).size / 1024).toFixed(1)} KB)`)
  console.log(`\nFor local HelixBLAST, set in databases.yaml:
  transcript:
    index_path: ${gzPath}\n\nR2 only needs FASTA files (index lives on HelixBLAST server):
  ./worker/scripts/split_fasta.sh ${fastaFile} output_dir/
  for f in output_dir/*.fa.gz; do
    wrangler r2 object put "helixblast-genomes/fasta/${outputPrefix}/$(basename $f)" --file "$f"
  done`)
}

main().catch(err => {
  console.error(err)
  process.exit(1)
})
