import { useEffect, useRef, useState } from 'react'
import type { Pipeline } from '../types'
import { MetricsGraph } from './MetricsGraph'

interface Props {
  pipeline: Pipeline
  onEdit: () => void
  onBack: () => void
}

export function PipelineDetail({ pipeline, onEdit, onBack }: Props) {
  const [logs, setLogs] = useState<string[]>([])
  const [wsState, setWsState] = useState<'connecting' | 'open' | 'closed'>('connecting')
  const [paused, setPaused] = useState(false)
  const pausedRef = useRef(false)
  const logEndRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const { namespace, name } = pipeline.metadata
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const url = `${proto}//${window.location.host}/api/v1/namespaces/${namespace}/pipelines/${name}/logs`
    const ws = new WebSocket(url)

    ws.onopen = () => setWsState('open')
    ws.onmessage = e => {
      if (!pausedRef.current) setLogs(prev => [...prev.slice(-499), e.data as string])
    }
    ws.onclose = () => setWsState('closed')
    ws.onerror = () => setWsState('closed')

    return () => ws.close()
  }, [pipeline.metadata.namespace, pipeline.metadata.name])

  useEffect(() => {
    if (!paused) logEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [logs, paused])

  function togglePause() {
    pausedRef.current = !pausedRef.current
    setPaused(p => !p)
  }

  const p = pipeline
  return (
    <div>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 20 }}>
        <button onClick={onBack} style={backBtnStyle}>← Zurück</button>
        <h2 style={{ margin: 0, fontSize: 18 }}>{p.metadata.name}</h2>
        <span style={{ color: '#888', fontSize: 13 }}>{p.metadata.namespace}</span>
        <PhaseBadge phase={p.status?.phase} />
        <div style={{ marginLeft: 'auto' }}>
          <button onClick={onEdit} style={editBtnStyle}>Bearbeiten</button>
        </div>
      </div>

      {/* Metadata */}
      <div style={sectionStyle}>
        <div style={metaRowStyle}>
          <span style={metaLabelStyle}>Pod</span>
          <code style={{ fontSize: 12 }}>{p.status?.podName ?? '—'}</code>
        </div>
        <div style={metaRowStyle}>
          <span style={metaLabelStyle}>Phase</span>
          <span>{p.status?.phase ?? '—'}</span>
        </div>
      </div>

      {/* Conditions */}
      {p.status?.conditions && p.status.conditions.length > 0 && (
        <div style={sectionStyle}>
          <h3 style={sectionTitleStyle}>Conditions</h3>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
            <thead>
              <tr style={{ background: '#f5f5f5' }}>
                <th style={thStyle}>Type</th>
                <th style={thStyle}>Status</th>
                <th style={thStyle}>Reason</th>
                <th style={thStyle}>Message</th>
              </tr>
            </thead>
            <tbody>
              {p.status.conditions.map(c => (
                <tr key={c.type} style={{ borderBottom: '1px solid #eee' }}>
                  <td style={tdStyle}>{c.type}</td>
                  <td style={tdStyle}>{c.status}</td>
                  <td style={tdStyle}>{c.reason ?? '—'}</td>
                  <td style={{ ...tdStyle, color: '#555' }}>{c.message ?? '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Metrics */}
      <MetricsGraph
        namespace={p.metadata.namespace}
        pipelineName={p.metadata.name}
        podName={p.status?.podName ?? ''}
        isRunning={p.status?.phase === 'Running'}
      />

      {/* Live Logs */}
      <div style={sectionStyle}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
          <h3 style={{ ...sectionTitleStyle, margin: 0 }}>Live-Logs</h3>
          <span style={wsStateBadgeStyle(wsState)}>{wsState}</span>
          {wsState === 'open' && (
            <button onClick={togglePause} title={paused ? 'Fortsetzen' : 'Pausieren'}
              style={{ border: 'none', background: 'none', cursor: 'pointer',
                       fontSize: 14, padding: '1px 4px' }}>
              {paused ? '▶' : '⏸'}
            </button>
          )}
          {logs.length > 0 && (
            <button
              onClick={() => setLogs([])}
              style={{ marginLeft: 'auto', fontSize: 12, color: '#888',
                       border: 'none', background: 'none', cursor: 'pointer' }}
            >
              Leeren
            </button>
          )}
        </div>
        <pre style={logPanelStyle}>
          {logs.length === 0
            ? <span style={{ color: '#888' }}>
                {wsState === 'connecting' ? 'Verbinde…' :
                 wsState === 'closed' ? 'Keine Logs (Pod läuft möglicherweise nicht).' :
                 'Warte auf Log-Ausgabe…'}
              </span>
            : logs.join('\n')
          }
          <div ref={logEndRef} />
        </pre>
      </div>
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
    <span style={{ background: c.bg, color: c.text, padding: '2px 8px',
                   borderRadius: 10, fontSize: 12, fontWeight: 600 }}>
      {phase ?? 'Unknown'}
    </span>
  )
}

function wsStateBadgeStyle(state: 'connecting' | 'open' | 'closed'): React.CSSProperties {
  const colors = {
    connecting: { color: '#d97706', background: '#fef9c3' },
    open:       { color: '#16a34a', background: '#dcfce7' },
    closed:     { color: '#6b7280', background: '#f3f4f6' },
  }
  return { ...colors[state], padding: '1px 7px', borderRadius: 8, fontSize: 11 }
}

const sectionStyle: React.CSSProperties = {
  background: '#fafafa', border: '1px solid #eee', borderRadius: 6,
  padding: 16, marginBottom: 16,
}
const sectionTitleStyle: React.CSSProperties = { fontSize: 14, fontWeight: 600, marginBottom: 10 }
const metaRowStyle: React.CSSProperties = { display: 'flex', gap: 12, marginBottom: 4 }
const metaLabelStyle: React.CSSProperties = { color: '#888', fontSize: 13, width: 60 }
const thStyle: React.CSSProperties = { padding: '6px 10px', textAlign: 'left', fontWeight: 600 }
const tdStyle: React.CSSProperties = { padding: '6px 10px' }
const backBtnStyle: React.CSSProperties = {
  border: 'none', background: 'none', cursor: 'pointer', fontSize: 14, color: '#3b82f6',
}
const editBtnStyle: React.CSSProperties = {
  padding: '5px 14px', border: '1px solid #ccc', borderRadius: 4,
  background: 'none', cursor: 'pointer', fontSize: 13,
}
const logPanelStyle: React.CSSProperties = {
  background: '#1e1e1e', color: '#d4d4d4', padding: 12, borderRadius: 4,
  fontSize: 12, lineHeight: 1.5, overflow: 'auto', maxHeight: 400,
  margin: 0, fontFamily: 'monospace',
}
