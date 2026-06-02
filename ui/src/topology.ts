import type { PipelineProject, ProjectRoute } from './types'

export type NodeKind = 'pipeline' | 'router'

export interface TopoNode {
  id: string          // pipeline name, or `route:<name>` for a router
  kind: NodeKind
  label: string       // display label (pipeline name or route name)
  routeName?: string  // set on router nodes
  layer: number       // assigned by computeLayout
  x: number
  y: number
}

export interface TopoEdge {
  id: string
  from: string        // node id
  to: string          // node id
  /** Consumer-side predicate, only on router→pipeline edges. */
  predicate?: string
}

export interface Topology {
  nodes: TopoNode[]
  edges: TopoEdge[]
  width: number
  height: number
}

// Layout constants (px). Exported so the canvas can size node boxes to match.
export const NODE_W = 150
export const NODE_H = 48
export const COL_GAP = 90
export const ROW_GAP = 28

const routerId = (route: string) => `route:${route}`

/** Build the node/edge graph from a project's routes. Positions are 0 until computeLayout runs. */
export function buildTopology(project: PipelineProject): Topology {
  const routes: ProjectRoute[] = project.spec.routes ?? []
  const nodes = new Map<string, TopoNode>()
  const edges: TopoEdge[] = []

  const ensurePipeline = (name: string) => {
    if (!name) return
    if (!nodes.has(name)) {
      nodes.set(name, { id: name, kind: 'pipeline', label: name, layer: 0, x: 0, y: 0 })
    }
  }

  for (const r of routes) {
    const rid = routerId(r.name)
    nodes.set(rid, { id: rid, kind: 'router', label: r.name, routeName: r.name, layer: 0, x: 0, y: 0 })
    ensurePipeline(r.from)
    edges.push({ id: `${r.from}->${rid}`, from: r.from, to: rid })
    for (const t of r.to ?? []) {
      ensurePipeline(t.pipeline)
      edges.push({ id: `${rid}->${t.pipeline}`, from: rid, to: t.pipeline, predicate: t.when || undefined })
    }
  }

  return { nodes: [...nodes.values()], edges, width: 0, height: 0 }
}

/** Assign layers by longest path from any source, then x/y grid positions. Pure. */
export function computeLayout(topo: Topology): Topology {
  const outgoing = new Map<string, string[]>()
  const indeg = new Map<string, number>()
  for (const n of topo.nodes) { outgoing.set(n.id, []); indeg.set(n.id, 0) }
  for (const e of topo.edges) {
    outgoing.get(e.from)?.push(e.to)
    indeg.set(e.to, (indeg.get(e.to) ?? 0) + 1)
  }

  // Longest-path layering via Kahn topological order. Robust to the acyclic
  // graphs admission guarantees; if a cycle slips through, nodes default to 0.
  const layer = new Map<string, number>(topo.nodes.map(n => [n.id, 0]))
  const queue = topo.nodes.filter(n => (indeg.get(n.id) ?? 0) === 0).map(n => n.id)
  const remaining = new Map(indeg)
  while (queue.length > 0) {
    const id = queue.shift()!
    const base = layer.get(id) ?? 0
    for (const next of outgoing.get(id) ?? []) {
      layer.set(next, Math.max(layer.get(next) ?? 0, base + 1))
      remaining.set(next, (remaining.get(next) ?? 0) - 1)
      if ((remaining.get(next) ?? 0) === 0) queue.push(next)
    }
  }

  // Group by layer, assign y within each layer in stable insertion order.
  const layers = new Map<number, TopoNode[]>()
  for (const n of topo.nodes) {
    n.layer = layer.get(n.id) ?? 0
    if (!layers.has(n.layer)) layers.set(n.layer, [])
    layers.get(n.layer)!.push(n)
  }

  let maxLayer = 0
  let maxRows = 0
  for (const [lyr, group] of layers) {
    maxLayer = Math.max(maxLayer, lyr)
    maxRows = Math.max(maxRows, group.length)
    group.forEach((n, row) => {
      n.x = lyr * (NODE_W + COL_GAP)
      n.y = row * (NODE_H + ROW_GAP)
    })
  }

  const width = (maxLayer + 1) * NODE_W + maxLayer * COL_GAP
  const height = maxRows * NODE_H + Math.max(0, maxRows - 1) * ROW_GAP
  return { ...topo, width, height }
}
