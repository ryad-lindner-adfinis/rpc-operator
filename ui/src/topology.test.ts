import { describe, it, expect } from 'vitest'
import { buildTopology, computeLayout } from './topology'
import type { PipelineProject } from './types'

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
