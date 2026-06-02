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
      </defs>

      {/* edges first so boxes paint on top */}
      {topology.edges.map(e => {
        const a = pos.get(e.from); const b = pos.get(e.to)
        if (!a || !b) return null
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

      {topology.nodes.map(n => (
        <NodeBox key={n.id} node={n} selected={n.id === selectedId} onSelect={onSelect} />
      ))}
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

function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n - 1) + '…' : s
}
