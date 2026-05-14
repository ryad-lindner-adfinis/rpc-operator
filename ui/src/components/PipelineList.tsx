import { useEffect, useState } from 'react'
import { listPipelines, deletePipeline } from '../api'
import type { Pipeline } from '../types'

interface Props {
  namespace: string
  onEdit: (pipeline: Pipeline) => void
  onNew: () => void
}

export function PipelineList({ namespace, onEdit, onNew }: Props) {
  const [pipelines, setPipelines] = useState<Pipeline[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string>()

  function load() {
    listPipelines(namespace)
      .then(items => { setPipelines(items); setError(undefined) })
      .catch(e => setError((e as Error).message))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    load()
    const id = setInterval(load, 10_000)
    return () => clearInterval(id)
  }, [namespace])

  async function handleDelete(p: Pipeline, e: React.MouseEvent) {
    e.stopPropagation()
    if (!confirm(`Pipeline "${p.metadata.name}" löschen?`)) return
    await deletePipeline(p.metadata.namespace, p.metadata.name).catch(console.error)
    load()
  }

  if (loading) return <p style={{ color: '#888' }}>Lade Pipelines…</p>
  if (error)   return <p style={{ color: 'red' }}>Fehler: {error}</p>

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <h2 style={{ margin: 0, fontSize: 18 }}>Pipelines — {namespace}</h2>
        <button onClick={onNew} style={newBtnStyle}>+ Neue Pipeline</button>
      </div>
      {pipelines.length === 0 ? (
        <p style={{ color: '#888' }}>Keine Pipelines in diesem Namespace.</p>
      ) : (
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14 }}>
          <thead>
            <tr style={{ background: '#f5f5f5', textAlign: 'left' }}>
              <th style={thStyle}>Name</th>
              <th style={thStyle}>Status</th>
              <th style={thStyle}>Pod</th>
              <th style={thStyle}>Letztes Update</th>
              <th style={thStyle}></th>
            </tr>
          </thead>
          <tbody>
            {pipelines.map(p => (
              <tr
                key={p.metadata.name}
                onClick={() => onEdit(p)}
                style={{ cursor: 'pointer', borderBottom: '1px solid #eee' }}
                onMouseEnter={e => (e.currentTarget.style.background = '#f9f9ff')}
                onMouseLeave={e => (e.currentTarget.style.background = '')}
              >
                <td style={tdStyle}><strong>{p.metadata.name}</strong></td>
                <td style={tdStyle}><PhaseBadge phase={p.status?.phase} /></td>
                <td style={{ ...tdStyle, color: '#666', fontFamily: 'monospace', fontSize: 12 }}>
                  {p.status?.podName ?? '—'}
                </td>
                <td style={{ ...tdStyle, color: '#666', fontSize: 12 }}>
                  {lastUpdated(p)}
                </td>
                <td style={{ ...tdStyle, textAlign: 'right' }}>
                  <button
                    onClick={e => handleDelete(p, e)}
                    style={{ color: '#c00', border: 'none', background: 'none', cursor: 'pointer', fontSize: 13 }}
                  >
                    Löschen
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

function PhaseBadge({ phase }: { phase?: string }) {
  const colors: Record<string, { bg: string; text: string }> = {
    Running: { bg: '#dcfce7', text: '#16a34a' },
    Failed:  { bg: '#fee2e2', text: '#dc2626' },
    Pending: { bg: '#fef9c3', text: '#d97706' },
    Stopped: { bg: '#f3f4f6', text: '#6b7280' },
  }
  const c = colors[phase ?? ''] ?? { bg: '#f3f4f6', text: '#6b7280' }
  return (
    <span style={{
      background: c.bg, color: c.text,
      padding: '2px 8px', borderRadius: 12, fontSize: 12, fontWeight: 600,
    }}>
      {phase ?? 'Unknown'}
    </span>
  )
}

function lastUpdated(p: Pipeline): string {
  const times = (p.status?.conditions ?? [])
    .map(c => c.lastTransitionTime)
    .filter(Boolean) as string[]
  const ts = times.length > 0
    ? times.reduce((a, b) => (a > b ? a : b))
    : p.metadata.creationTimestamp
  if (!ts) return '—'
  return new Date(ts).toLocaleString('de-DE', { dateStyle: 'short', timeStyle: 'short' })
}

const thStyle: React.CSSProperties = { padding: '8px 12px', fontWeight: 600, fontSize: 13 }
const tdStyle: React.CSSProperties = { padding: '10px 12px', verticalAlign: 'middle' }
const newBtnStyle: React.CSSProperties = {
  padding: '6px 16px', background: '#3b82f6', color: '#fff',
  border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 14,
}
