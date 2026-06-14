import { describe, it, expect } from 'vitest'
import { buildTopology, computeLayout } from './topology'
import type { PipelineProject } from './types'
import type { CacheUse } from './cacheUsage'

function proj(routes: PipelineProject['spec']['routes']): PipelineProject {
  return { metadata: { name: 'orders', namespace: 'default' }, spec: { routes } }
}

describe('buildTopology', () => {
  it('creates pipeline + router nodes and edges for a fan-out route', () => {
    const t = buildTopology(proj([
      { name: 'fan', from: 'ingest', to: [{ pipeline: 'warehouse' }, { pipeline: 'alert', when: 'this.level == "high"' }] },
    ]))
    // pipeline nodes: ingest, warehouse, alert ; router node: fan
    expect(t.nodes.filter(n => n.kind === 'pipeline').map(n => n.id).sort())
      .toEqual(['alert', 'ingest', 'warehouse'])
    expect(t.nodes.filter(n => n.kind === 'router').map(n => n.id)).toEqual(['route:fan'])
    // edges: ingest->router, router->warehouse, router->alert(with predicate)
    expect(t.edges).toHaveLength(3)
    const alertEdge = t.edges.find(e => e.to === 'alert')
    expect(alertEdge?.predicate).toBe('this.level == "high"')
  })

  it('adds projectRef members not referenced by any route as standalone nodes', () => {
    const t = buildTopology(proj([
      { name: 'fan', from: 'ingest', to: [{ pipeline: 'warehouse' }] },
    ]), ['ingest', 'warehouse', 'lonely'])
    const pipes = t.nodes.filter(n => n.kind === 'pipeline').map(n => n.id).sort()
    // ingest/warehouse already exist via the route; lonely is added standalone, no dupes.
    expect(pipes).toEqual(['ingest', 'lonely', 'warehouse'])
    // the standalone member has no edges.
    expect(t.edges.some(e => e.from === 'lonely' || e.to === 'lonely')).toBe(false)
  })

  it('renders members even when the project has no routes at all', () => {
    const t = buildTopology(proj([]), ['solo'])
    expect(t.nodes).toHaveLength(1)
    expect(t.nodes[0]).toMatchObject({ id: 'solo', kind: 'pipeline' })
  })

  it('deduplicates a pipeline that is both a source and a target', () => {
    const t = buildTopology(proj([
      { name: 'a', from: 'p1', to: [{ pipeline: 'p2' }] },
      { name: 'b', from: 'p2', to: [{ pipeline: 'p3' }] },
    ]))
    const pipes = t.nodes.filter(n => n.kind === 'pipeline').map(n => n.id).sort()
    expect(pipes).toEqual(['p1', 'p2', 'p3']) // p2 appears once
  })
})

describe('computeLayout', () => {
  it('assigns increasing x by layer (source < router < sink)', () => {
    const t = computeLayout(buildTopology(proj([
      { name: 'fan', from: 'ingest', to: [{ pipeline: 'warehouse' }] },
    ])))
    const x = (id: string) => t.nodes.find(n => n.id === id)!.x
    expect(x('ingest')).toBeLessThan(x('route:fan'))
    expect(x('route:fan')).toBeLessThan(x('warehouse'))
  })

  it('gives every node a non-negative y and no two nodes in a layer share a y', () => {
    const t = computeLayout(buildTopology(proj([
      { name: 'fan', from: 'ingest', to: [{ pipeline: 'warehouse' }, { pipeline: 'alert' }] },
    ])))
    const sinks = t.nodes.filter(n => n.id === 'warehouse' || n.id === 'alert')
    expect(sinks[0].y).not.toBe(sinks[1].y)
    expect(t.nodes.every(n => n.y >= 0)).toBe(true)
    expect(t.width).toBeGreaterThan(0)
    expect(t.height).toBeGreaterThan(0)
  })
})

describe('buildTopology with caches', () => {
  function projWithCaches(): PipelineProject {
    return {
      metadata: { name: 'orders', namespace: 'default' },
      spec: {
        routes: [{ name: 'fan', from: 'ingest', to: [{ pipeline: 'warehouse' }] }],
        cacheResources: [{ name: 'shared', natsKV: {} }, { name: 'unused', natsKV: {} }],
      },
    }
  }

  it('adds a cache node per declared cache and a dashed edge per use', () => {
    const t = buildTopology(projWithCaches(), ['ingest', 'warehouse'],
      [{ pipeline: 'warehouse', cache: 'shared', operators: ['get', 'set'] }])
    expect(t.nodes.filter(n => n.kind === 'cache').map(n => n.id).sort())
      .toEqual(['cache:shared', 'cache:unused'])
    const edge = t.edges.find(e => e.kind === 'cache')
    expect(edge).toMatchObject({ from: 'warehouse', to: 'cache:shared', operators: ['get', 'set'] })
  })

  it('marks a used-but-undeclared cache as a phantom node', () => {
    const t = buildTopology(projWithCaches(), ['ingest', 'warehouse'],
      [{ pipeline: 'warehouse', cache: 'ghost', operators: ['get'] }])
    const ghost = t.nodes.find(n => n.id === 'cache:ghost')
    expect(ghost).toMatchObject({ kind: 'cache', cacheName: 'ghost', undeclared: true })
  })

  it('does not let cache edges change route layering', () => {
    const t = computeLayout(buildTopology(projWithCaches(), ['ingest', 'warehouse'],
      [{ pipeline: 'warehouse', cache: 'shared', operators: ['get'] }]))
    const x = (id: string) => t.nodes.find(n => n.id === id)!.x
    // ingest -> router -> warehouse layering is unchanged.
    expect(x('ingest')).toBeLessThan(x('route:fan'))
    expect(x('route:fan')).toBeLessThan(x('warehouse'))
  })

  it('places cache nodes in a band below the route DAG', () => {
    const t = computeLayout(buildTopology(projWithCaches(), ['ingest', 'warehouse'],
      [{ pipeline: 'warehouse', cache: 'shared', operators: ['get'] }]))
    const routeMaxY = Math.max(...t.nodes.filter(n => n.kind !== 'cache').map(n => n.y))
    const cacheY = t.nodes.find(n => n.id === 'cache:shared')!.y
    expect(cacheY).toBeGreaterThan(routeMaxY)
  })
})
