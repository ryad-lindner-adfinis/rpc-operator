import yaml from 'js-yaml'
import type { Pipeline } from './types'

export interface CacheUse {
  pipeline: string
  cache: string
  operators: string[]
}

/**
 * Recursively collect (resource, operator) pairs from `cache:` mappings.
 * Two shapes are recognised:
 *   - the `cache` processor — has BOTH `resource` and `operator` strings;
 *     recorded with its real operator (get/set/add/delete/exists).
 *   - the `cache` output — has a `target` string (no operator); recorded with
 *     the synthetic operator 'output', the cache being its write-sink.
 * The cache input (no `target`, no `operator`) and the `cached` processor
 * (`cache` is a string) both fail these tests and are skipped.
 */
function collect(node: unknown, out: Array<{ resource: string; operator: string }>): void {
  if (Array.isArray(node)) {
    for (const item of node) collect(item, out)
    return
  }
  if (node && typeof node === 'object') {
    const obj = node as Record<string, unknown>
    const c = obj.cache
    if (c && typeof c === 'object' && !Array.isArray(c)) {
      const cc = c as Record<string, unknown>
      if (typeof cc.resource === 'string' && typeof cc.operator === 'string') {
        out.push({ resource: cc.resource, operator: cc.operator })
      } else if (typeof cc.target === 'string') {
        out.push({ resource: cc.target, operator: 'output' })
      }
    }
    for (const v of Object.values(obj)) collect(v, out)
  }
}

/**
 * Detect which pipelines use which cache resource, and with which operators.
 * Only rawYAML pipelines are scanned: the component catalog has no `cache`
 * processor, so structured pipelines cannot reference a cache.
 */
export function detectCacheUses(pipelines: Pipeline[]): CacheUse[] {
  const grouped = new Map<string, Map<string, Set<string>>>() // pipeline -> cache -> operators
  for (const p of pipelines) {
    const rawYAML = p.spec.rawYAML
    if (!rawYAML) continue
    let doc: unknown
    try {
      doc = yaml.load(rawYAML)
    } catch {
      continue
    }
    const pairs: Array<{ resource: string; operator: string }> = []
    collect(doc, pairs)
    if (pairs.length === 0) continue
    let byCache = grouped.get(p.metadata.name)
    if (!byCache) { byCache = new Map(); grouped.set(p.metadata.name, byCache) }
    for (const { resource, operator } of pairs) {
      let ops = byCache.get(resource)
      if (!ops) { ops = new Set(); byCache.set(resource, ops) }
      ops.add(operator)
    }
  }
  const uses: CacheUse[] = []
  for (const [pipeline, byCache] of grouped) {
    for (const [cache, ops] of byCache) {
      const sorted = [...ops].filter(o => o !== 'output').sort()
      if (ops.has('output')) sorted.push('output')
      uses.push({ pipeline, cache, operators: sorted })
    }
  }
  return uses
}
