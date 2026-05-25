import type { ClusterDistribution } from '../types'

interface Props {
  distribution: ClusterDistribution
  onOpenPipeline: (name: string) => void
}

export function ClusterInstances({ distribution, onOpenPipeline }: Props) {
  const { instances, stalePlacements } = distribution
  return (
    <div style={sectionStyle}>
      <h3 style={titleStyle}>Instances</h3>
      {instances.length === 0 ? (
        <p style={{ color: '#888', fontSize: 13, margin: 0 }}>No instances reported.</p>
      ) : (
        instances.map(inst => (
          <div key={inst.name} style={instanceRowStyle}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <span style={{ color: inst.ready ? '#16a34a' : '#9ca3af', fontSize: 16, lineHeight: 1 }}>●</span>
              <code style={{ fontSize: 13 }}>{inst.name}</code>
              <span style={{ fontSize: 12, color: '#888' }}>
                {inst.ready ? 'ready' : 'not ready'} · {inst.assignedPipelines.length} stream(s)
              </span>
            </div>
            {inst.assignedPipelines.length > 0 && (
              <div style={chipsRowStyle}>
                {inst.assignedPipelines.map(name => (
                  <button key={name} onClick={() => onOpenPipeline(name)} style={chipStyle}>
                    {name}
                  </button>
                ))}
              </div>
            )}
          </div>
        ))
      )}

      {stalePlacements.length > 0 && (
        <div style={staleBoxStyle}>
          <div style={{ fontSize: 12, fontWeight: 600, color: '#92400e', marginBottom: 4 }}>
            Stale placements (assigned instance no longer exists)
          </div>
          {stalePlacements.map(sp => (
            <div key={sp.pipeline} style={{ fontSize: 12, color: '#92400e' }}>
              <button onClick={() => onOpenPipeline(sp.pipeline)} style={staleLinkStyle}>{sp.pipeline}</button>
              {' → '}<code>{sp.assignedInstance}</code>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

const sectionStyle: React.CSSProperties = {
  background: '#fafafa', border: '1px solid #eee', borderRadius: 6, padding: 16, marginBottom: 16,
}
const titleStyle: React.CSSProperties = { fontSize: 14, fontWeight: 600, margin: '0 0 12px' }
const instanceRowStyle: React.CSSProperties = {
  padding: '8px 0', borderBottom: '1px solid #f0f0f0',
}
const chipsRowStyle: React.CSSProperties = {
  display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 6, paddingLeft: 24,
}
const chipStyle: React.CSSProperties = {
  background: '#eef2ff', color: '#3730a3', border: '1px solid #e0e7ff',
  borderRadius: 4, padding: '2px 8px', fontSize: 12, cursor: 'pointer',
}
const staleBoxStyle: React.CSSProperties = {
  background: '#fffbeb', border: '1px solid #fde68a', borderRadius: 4,
  padding: '8px 12px', marginTop: 12,
}
const staleLinkStyle: React.CSSProperties = {
  background: 'none', border: 'none', padding: 0, color: '#92400e',
  textDecoration: 'underline', cursor: 'pointer', fontSize: 12,
}
