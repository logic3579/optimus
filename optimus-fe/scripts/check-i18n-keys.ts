/* eslint-disable @typescript-eslint/no-explicit-any */
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const RE = /(?:\$t|i18n\.t|\bt)\(\s*['"`]([^'"`\s)]+)['"`]/g

function walk(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const p = path.join(dir, name)
    if (statSync(p).isDirectory()) walk(p, out)
    else if (/\.(vue|ts)$/.test(p) && !p.endsWith('.test.ts')) out.push(p)
  }
  return out
}

export function extractUsedKeys(_path: string, src: string): string[] {
  const out = new Set<string>()
  for (const m of src.matchAll(RE)) {
    if (m[1]) out.add(m[1])
  }
  return [...out]
}

export function flattenKeys(obj: Record<string, unknown>, prefix = ''): string[] {
  const out: string[] = []
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object' && !Array.isArray(v)) {
      out.push(...flattenKeys(v as Record<string, unknown>, key))
    } else {
      out.push(key)
    }
  }
  return out
}

export interface AuditInput {
  sources: Array<{ path: string; usedKeys: string[] }>
  zhCN: Record<string, unknown>
  enUS: Record<string, unknown>
}
export interface AuditResult {
  missingFromZh: string[]
  zhEnMismatch: string[]
  unused: string[]
}

export function auditKeys(input: AuditInput): AuditResult {
  const zhFlat = new Set(flattenKeys(input.zhCN))
  const enFlat = new Set(flattenKeys(input.enUS))
  const used = new Set<string>()
  for (const s of input.sources) for (const k of s.usedKeys) used.add(k)

  const missingFromZh: string[] = []
  for (const k of used) if (!zhFlat.has(k)) missingFromZh.push(k)

  const zhEnMismatch: string[] = []
  for (const k of zhFlat) if (!enFlat.has(k)) zhEnMismatch.push(k)
  for (const k of enFlat) if (!zhFlat.has(k)) zhEnMismatch.push(k)

  const unused: string[] = []
  for (const k of zhFlat) if (!used.has(k) && !k.startsWith('menu.') && !k.startsWith('perm.')) {
    unused.push(k)
  }

  return { missingFromZh, zhEnMismatch, unused }
}

// CLI entry — only runs when executed directly, not when imported by tests.
async function main() {
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
  const files = walk(`${root}/src`)
  const sources = files.map(p => ({
    path: p,
    usedKeys: extractUsedKeys(p, readFileSync(p, 'utf8'))
  }))
  const zhCN = JSON.parse(readFileSync(`${root}/src/locales/zh-CN.json`, 'utf8'))
  const enUS = JSON.parse(readFileSync(`${root}/src/locales/en-US.json`, 'utf8'))
  const r = auditKeys({ sources, zhCN, enUS })

  let fatal = false
  if (r.missingFromZh.length) {
    console.error('Missing keys in zh-CN.json:')
    for (const k of r.missingFromZh) console.error('  -', k)
    fatal = true
  }
  if (r.zhEnMismatch.length) {
    console.error('zh-CN vs en-US key mismatch:')
    for (const k of r.zhEnMismatch) console.error('  -', k)
    fatal = true
  }
  if (r.unused.length) {
    console.warn(`Unused keys (warning only, ${r.unused.length}):`)
    for (const k of r.unused.slice(0, 10)) console.warn('  -', k)
    if (r.unused.length > 10) console.warn(`  … +${r.unused.length - 10} more`)
  }
  if (fatal) process.exit(1)
  console.log('i18n keys OK')
}

// Bun's import.meta.main exposes whether the file is the entrypoint.
if ((import.meta as any).main) {
  await main()
}
