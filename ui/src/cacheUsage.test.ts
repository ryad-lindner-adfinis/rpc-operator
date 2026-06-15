import { describe, it, expect } from 'vitest'
import { detectCacheUses } from './cacheUsage'
import type { Pipeline } from './types'

function raw(name: string, rawYAML: string): Pipeline {
  return { metadata: { name, namespace: 'default' }, spec: { rawYAML } } as Pipeline
}

describe('detectCacheUses', () => {
  it('detects a single cache processor with its operator', () => {
    const p = raw('enrich', `
pipeline:
  processors:
    - cache:
        resource: shared
        operator: get
        key: \${! json("id") }
`)
    expect(detectCacheUses([p])).toEqual([{ pipeline: 'enrich', cache: 'shared', operators: ['get'] }])
  })

  it('groups multiple operators on the same cache, sorted and de-duplicated', () => {
    const p = raw('rw', `
pipeline:
  processors:
    - cache: { resource: shared, operator: set, key: k }
    - cache: { resource: shared, operator: get, key: k }
    - cache: { resource: shared, operator: set, key: k2 }
`)
    expect(detectCacheUses([p])).toEqual([{ pipeline: 'rw', cache: 'shared', operators: ['get', 'set'] }])
  })

  it('finds cache processors nested under branch/for_each', () => {
    const p = raw('nested', `
pipeline:
  processors:
    - branch:
        processors:
          - cache: { resource: a, operator: exists, key: k }
    - for_each:
        - cache: { resource: b, operator: delete, key: k }
`)
    const uses = detectCacheUses([p]).sort((x, y) => x.cache.localeCompare(y.cache))
    expect(uses).toEqual([
      { pipeline: 'nested', cache: 'a', operators: ['exists'] },
      { pipeline: 'nested', cache: 'b', operators: ['delete'] },
    ])
  })

  it('ignores cache input (no operator, no target)', () => {
    const p = raw('in', `
input:
  cache: { resource: shared, key: k }
`)
    expect(detectCacheUses([p])).toEqual([])
  })

  it('detects a cache output via its target field', () => {
    const p = raw('sink', `
output:
  cache: { target: shared, key: k }
`)
    expect(detectCacheUses([p])).toEqual([{ pipeline: 'sink', cache: 'shared', operators: ['output'] }])
  })

  it('appends output last after sorted processor operators on the same cache', () => {
    const p = raw('rw', `
pipeline:
  processors:
    - cache: { resource: shared, operator: set, key: k }
output:
  cache: { target: shared, key: k }
`)
    expect(detectCacheUses([p])).toEqual([{ pipeline: 'rw', cache: 'shared', operators: ['set', 'output'] }])
  })

  it('finds a cache output nested under a broker output', () => {
    const p = raw('broker', `
output:
  broker:
    outputs:
      - cache: { target: b, key: k }
`)
    expect(detectCacheUses([p])).toEqual([{ pipeline: 'broker', cache: 'b', operators: ['output'] }])
  })

  it('ignores the cached processor (cache is a string)', () => {
    const p = raw('cachedproc', `
pipeline:
  processors:
    - cached:
        key: k
        cache: shared
        processors:
          - mapping: root = this
`)
    expect(detectCacheUses([p])).toEqual([])
  })

  it('skips structured pipelines (no rawYAML)', () => {
    const structured = { metadata: { name: 's', namespace: 'default' },
      spec: { processors: [{ type: 'mapping', config: 'root = this' }] } } as Pipeline
    expect(detectCacheUses([structured])).toEqual([])
  })

  it('skips pipelines with malformed YAML', () => {
    expect(detectCacheUses([raw('bad', 'pipeline: [unterminated')])).toEqual([])
  })
})
