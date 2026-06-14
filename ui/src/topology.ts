import type { PipelineProject, ProjectRoute } from './types'
import type { CacheUse } from './cacheUsage'

export type NodeKind = 'pipeline' | 'router' | 'cache'

export interface TopoNode {
  id: string          // pipeline name, `route:<name>` for a router, or `cache:<name>` for a cache
  kind: NodeKind
  label: string       // display label (pipeline name, route name, or cache name)
  routeName?: string  // set on router nodes
  cacheName?: string  // set on cache nodes
  undeclared?: boolean // cache used by a pipeline but not in spec.cacheResources
  layer: number       // assigned by computeLayout
  x: number
  y: number
}

export interface TopoEdge {
  id: string
  from: string        // node id
  to: string          // node id
  kind: 'route' | 'cache'
  /** Consumer-side predicate, only on router→pipeline edges. */
  predicate?: string
  /** Cache operators (get/set/add/delete/exists), only on cache edges. */
  operators?: string[]
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
export const BAND_GAP = 60   // vertical gap between the route DAG and the cache band

const routerId = (route: string) => `route:${route}`
const cacheId = (name: string) => `cache:${name}`

/**
 * Build the node/edge graph from a project's routes, member pipelines, and cache usages.
 * Positions are 0 until computeLayout runs.
 *
 * `memberPipelines` lists pipelines attached to the project via `projectRef`.
 * They are added as standalone nodes so a freshly-attached pipeline shows on the
 * map even before it is wired into any route (otherwise it would be invisible).
 *
 * `cacheUses` drives cache nodes and dashed cache edges. Cache edges carry
 * `kind: 'cache'` and are excluded from the route-layering algorithm.
 */
export function buildTopology(
  project: PipelineProject,
  memberPipelines: string[] = [],
  cacheUses: CacheUse[] = [],
): Topology {
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
    edges.push({ id: `${r.from}->${rid}`, from: r.from, to: rid, kind: 'route' })
    for (const t of r.to ?? []) {
      ensurePipeline(t.pipeline)
      edges.push({ id: `${rid}->${t.pipeline}`, from: rid, to: t.pipeline, kind: 'route', predicate: t.when || undefined })
    }
  }

  // Members not referenced by any route appear as unconnected pipeline nodes.
  for (const name of memberPipelines) ensurePipeline(name)

  // Declared cache resources become cache nodes.
  for (const c of project.spec.cacheResources ?? []) {
    nodes.set(cacheId(c.name), { id: cacheId(c.name), kind: 'cache', label: c.name, cacheName: c.name, layer: 0, x: 0, y: 0 })
  }

  // Cache usages: ensure the cache node exists (phantom if undeclared) and add a dashed edge.
  for (const u of cacheUses) {
    ensurePipeline(u.pipeline)
    const cid = cacheId(u.cache)
    if (!nodes.has(cid)) {
      nodes.set(cid, { id: cid, kind: 'cache', label: u.cache, cacheName: u.cache, undeclared: true, layer: 0, x: 0, y: 0 })
    }
    edges.push({ id: `${u.pipeline}~>${cid}`, from: u.pipeline, to: cid, kind: 'cache', operators: u.operators })
  }

  return { nodes: [...nodes.values()], edges, width: 0, height: 0 }
}

/** Assign layers by longest path over ROUTE edges, then x/y. Cache nodes go in a band below. Pure. */
export function computeLayout(topo: Topology): Topology {
  const routeNodes = topo.nodes.filter(n => n.kind !== 'cache')
  const cacheNodes = topo.nodes.filter(n => n.kind === 'cache')
  const routeEdges = topo.edges.filter(e => e.kind !== 'cache')

  const outgoing = new Map<string, string[]>()
  const indeg = new Map<string, number>()
  for (const n of routeNodes) { outgoing.set(n.id, []); indeg.set(n.id, 0) }
  for (const e of routeEdges) {
    outgoing.get(e.from)?.push(e.to)
    indeg.set(e.to, (indeg.get(e.to) ?? 0) + 1)
  }

  const layer = new Map<string, number>(routeNodes.map(n => [n.id, 0]))
  const queue = routeNodes.filter(n => (indeg.get(n.id) ?? 0) === 0).map(n => n.id)
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

  const layers = new Map<number, TopoNode[]>()
  for (const n of routeNodes) {
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

  const routeWidth = routeNodes.length ? (maxLayer + 1) * NODE_W + maxLayer * COL_GAP : 0
  const routeHeight = routeNodes.length ? maxRows * NODE_H + Math.max(0, maxRows - 1) * ROW_GAP : 0

  // Cache band below the route DAG, in declaration order.
  const bandY = routeHeight > 0 ? routeHeight + BAND_GAP : 0
  cacheNodes.forEach((n, i) => {
    n.layer = -1
    n.x = i * (NODE_W + COL_GAP)
    n.y = bandY
  })
  const bandWidth = cacheNodes.length ? cacheNodes.length * NODE_W + (cacheNodes.length - 1) * COL_GAP : 0

  const width = Math.max(routeWidth, bandWidth)
  const height = cacheNodes.length ? bandY + NODE_H : routeHeight
  return { ...topo, width, height }
}
