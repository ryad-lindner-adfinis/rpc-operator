import { NODE_W, NODE_H, type Topology, type TopoNode } from '../topology'

interface Props {
  topology: Topology
  selectedId: string | null
  onSelect: (id: string) => void
}

const PAD = 24

export function TopologyCanvas({ topology, selectedId, onSelect }: Props) {
  const w = topology.width + PAD * 2
  const h = Math.max(topology.height + PAD * 2, NODE_H + PAD * 2)
  const pos = new Map(topology.nodes.map(n => [n.id, n]))

  return (
    <svg width="100%" viewBox={`0 0 ${w} ${h}`} style={{ minHeight: 240, background: '#fcfcfd', borderRadius: 8 }}>
      <defs>
        <marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
          <path d="M0,0 L10,5 L0,10 z" fill="#94a3b8" />
        </marker>
        <marker id="cacheArrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
          <path d="M0,0 L10,5 L0,10 z" fill="#10b981" />
        </marker>
      </defs>

      {/* edges first so boxes paint on top */}
      {topology.edges.map(e => {
        const a = pos.get(e.from); const b = pos.get(e.to)
        if (!a || !b) return null

        if (e.kind === 'cache') {
          // pipeline (above) → cache (below): vertical-ish dashed bezier
          const x1 = a.x + NODE_W / 2 + PAD, y1 = a.y + NODE_H + PAD
          const x2 = b.x + NODE_W / 2 + PAD, y2 = b.y + PAD
          const my = (y1 + y2) / 2
          const d = `M ${x1},${y1} C ${x1},${my} ${x2},${my} ${x2},${y2}`
          return (
            <g key={e.id}>
              <path d={d} fill="none" stroke="#10b981" strokeWidth={1.5} strokeDasharray="5 4" markerEnd="url(#cacheArrow)" />
              {e.operators && e.operators.length > 0 && (
                <text x={(x1 + x2) / 2} y={my - 4} textAnchor="middle" fontSize={10} fill="#047857">
                  {e.operators.join(', ')}
                </text>
              )}
            </g>
          )
        }

        const x1 = a.x + NODE_W + PAD, y1 = a.y + NODE_H / 2 + PAD
        const x2 = b.x + PAD, y2 = b.y + NODE_H / 2 + PAD
        const mx = (x1 + x2) / 2
        const d = `M ${x1},${y1} C ${mx},${y1} ${mx},${y2} ${x2},${y2}`
        return (
          <g key={e.id}>
            <path d={d} fill="none" stroke="#94a3b8" strokeWidth={1.5} markerEnd="url(#arrow)" />
            {e.predicate && (
              <text x={mx} y={(y1 + y2) / 2 - 6} textAnchor="middle" fontSize={10} fill="#64748b">
                when:{truncate(e.predicate, 22)}
              </text>
            )}
          </g>
        )
      })}

      {topology.nodes.map(n => {
        if (n.kind !== 'cache') {
          return <NodeBox key={n.id} node={n} selected={n.id === selectedId} onSelect={onSelect} />
        }
        const used = topology.edges.some(e => e.kind === 'cache' && e.to === n.id)
        return (
          <CacheNode key={n.id} node={n} unused={!used && !n.undeclared}
                     selected={n.id === selectedId} onSelect={onSelect} />
        )
      })}
    </svg>
  )
}

function NodeBox({ node, selected, onSelect }: { node: TopoNode; selected: boolean; onSelect: (id: string) => void }) {
  const x = node.x + PAD, y = node.y + PAD
  const isRouter = node.kind === 'router'
  const fill = isRouter ? '#fef3c7' : '#eff6ff'
  const stroke = selected ? '#1d4ed8' : (isRouter ? '#f59e0b' : '#93c5fd')
  const rx = isRouter ? NODE_H / 2 : 8 // amber pill vs blue rectangle (spec)
  return (
    <g
      data-selected={selected}
      onClick={() => onSelect(node.id)}
      style={{ cursor: 'pointer' }}
    >
      <rect x={x} y={y} width={NODE_W} height={NODE_H} rx={rx}
            fill={fill} stroke={stroke} strokeWidth={selected ? 2.5 : 1.5} />
      <text x={x + NODE_W / 2} y={y + NODE_H / 2 + 4} textAnchor="middle"
            fontSize={13} fontWeight={600} fill={isRouter ? '#92400e' : '#1e3a8a'}>
        {truncate(node.label, 16)}
      </text>
    </g>
  )
}

function CacheNode({ node, unused, selected, onSelect }: {
  node: TopoNode; unused: boolean; selected: boolean; onSelect: (id: string) => void
}) {
  const x = node.x + PAD, y = node.y + PAD
  const undeclared = node.undeclared
  const fill = undeclared ? '#f9fafb' : (unused ? '#ecfdf5' : '#d1fae5')
  const stroke = selected ? '#1d4ed8' : (undeclared ? '#9ca3af' : (unused ? '#6ee7b7' : '#10b981'))
  const text = undeclared ? '#6b7280' : '#065f46'
  const subLabel = undeclared ? '⚠ undeclared' : (unused ? 'unused' : undefined)
  const ry = 9
  const top = y + ry
  const bottom = y + NODE_H - ry
  return (
    <g data-selected={selected} onClick={() => onSelect(node.id)} style={{ cursor: 'pointer' }}>
      <path d={`M${x},${top} L${x},${bottom} A${NODE_W / 2},${ry} 0 0 0 ${x + NODE_W},${bottom} L${x + NODE_W},${top}`}
            fill={fill} stroke={stroke} strokeWidth={selected ? 2.5 : 1.5}
            strokeDasharray={undeclared ? '4 3' : undefined} />
      <ellipse cx={x + NODE_W / 2} cy={top} rx={NODE_W / 2} ry={ry}
               fill={fill} stroke={stroke} strokeWidth={selected ? 2.5 : 1.5}
               strokeDasharray={undeclared ? '4 3' : undefined} />
      <text x={x + NODE_W / 2} y={y + NODE_H / 2 + 4} textAnchor="middle"
            fontSize={12} fontWeight={600} fill={text}>
        {truncate(node.label, 16)}
      </text>
      {subLabel && (
        <text x={x + NODE_W / 2} y={y + NODE_H + 12} textAnchor="middle" fontSize={9}
              fill={undeclared ? '#dc2626' : '#9ca3af'}>
          {subLabel}
        </text>
      )}
    </g>
  )
}

function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n - 1) + '…' : s
}
