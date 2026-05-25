import { useEffect, useRef, useState } from 'react'
import type { Pipeline } from '../types'
import { getToken } from '../auth'
import { getMetrics } from '../api'
import { MetricsGraph } from './MetricsGraph'

interface Props {
  pipeline: Pipeline
  /** F42 Mode C: hides the Edit, Stop, and Run buttons. */
  readOnly?: boolean
  /** F42: when false, skip the log WebSocket entirely and hide the Live-Logs section. */
  showLogs?: boolean
  onEdit: () => void
  onBack: () => void
  /** F45: stop a Running/Pending pipeline. Hidden when not provided. */
  onStop?: () => void
  /** F45: re-run a Stopped pipeline. Hidden when not provided. */
  onRun?: () => void
  /** F47 Phase 3c: jump to the assigned cluster's detail. */
  onOpenCluster?: (name: string) => void
}

export function PipelineDetail({
  pipeline, readOnly = false, showLogs = true,
  onEdit, onBack, onStop, onRun, onOpenCluster,
}: Props) {
  const [logs, setLogs] = useState<string[]>([])
  const [wsState, setWsState] = useState<'connecting' | 'open' | 'closed'>('connecting')
  const [paused, setPaused] = useState(false)
  const pausedRef = useRef(false)
  const logEndRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!showLogs) return
    const { namespace, name } = pipeline.metadata
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const token = getToken()
    const tokenQuery = token ? `?token=${encodeURIComponent(token)}` : ''
    const url = `${proto}//${window.location.host}/api/v1/namespaces/${namespace}/pipelines/${name}/logs${tokenQuery}`
    const ws = new WebSocket(url)

    ws.onopen = () => setWsState('open')
    ws.onmessage = e => {
      if (!pausedRef.current) setLogs(prev => [...prev.slice(-499), e.data as string])
    }
    ws.onclose = () => setWsState('closed')
    ws.onerror = () => setWsState('closed')

    return () => ws.close()
  }, [pipeline.metadata.namespace, pipeline.metadata.name, showLogs])

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
        <button onClick={onBack} style={backBtnStyle}>← Back</button>
        <h2 style={{ margin: 0, fontSize: 18 }}>{p.metadata.name}</h2>
        <span style={{ color: '#888', fontSize: 13 }}>{p.metadata.namespace}</span>
        <PhaseBadge phase={p.status?.phase} />
        <div style={{ marginLeft: 'auto', display: 'flex', gap: 8 }}>
          {!readOnly && onStop && (p.status?.phase === 'Running' || p.status?.phase === 'Pending') && (
            <button onClick={onStop} style={stopBtnStyle}>Stop</button>
          )}
          {!readOnly && onRun && p.status?.phase === 'Stopped' && (
            <button onClick={onRun} style={runBtnStyle}>Run</button>
          )}
          {!readOnly && (
            <button onClick={onEdit} style={editBtnStyle}>Edit</button>
          )}
        </div>
      </div>

      {/* Metadata */}
      <div style={sectionStyle}>
        {p.status?.assignedInstance ? (
          <div style={metaRowStyle}>
            <span style={metaLabelStyle}>Placement</span>
            <span style={{ fontSize: 13 }}>
              Cluster:{' '}
              {onOpenCluster && p.status.assignedCluster ? (
                <button onClick={() => onOpenCluster!(p.status!.assignedCluster!)} style={linkBtnStyle}>
                  {p.status.assignedCluster}
                </button>
              ) : (
                <code style={{ fontSize: 12 }}>{p.status.assignedCluster ?? '—'}</code>
              )}
              {' · '}Instance: <code style={{ fontSize: 12 }}>{p.status.assignedInstance}</code>
            </span>
          </div>
        ) : (
          <div style={metaRowStyle}>
            <span style={metaLabelStyle}>Pod</span>
            <code style={{ fontSize: 12 }}>{p.status?.podName ?? '—'}</code>
          </div>
        )}
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
        fetchMetrics={(q, start, end) => getMetrics(p.metadata.namespace, p.metadata.name, q, start, end)}
        isRunning={p.status?.phase === 'Running'}
      />

      {/* Live Logs — hidden in Mode C when anonymous.logs.enabled is false */}
      {showLogs && (
      <div style={sectionStyle}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
          <h3 style={{ ...sectionTitleStyle, margin: 0 }}>Live Logs</h3>
          <span style={wsStateBadgeStyle(wsState)}>{wsState}</span>
          {wsState === 'open' && (
            <button onClick={togglePause} title={paused ? 'Resume' : 'Pause'}
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
              Clear
            </button>
          )}
        </div>
        <pre style={logPanelStyle}>
          {logs.length === 0
            ? <span style={{ color: '#888' }}>
                {wsState === 'connecting' ? 'Connecting…' :
                 wsState === 'closed' ? 'No logs (pod may not be running).' :
                 'Waiting for log output…'}
              </span>
            : logs.join('\n')
          }
          <div ref={logEndRef} />
        </pre>
      </div>
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
const linkBtnStyle: React.CSSProperties = {
  border: 'none', background: 'none', padding: 0, color: '#3b82f6',
  textDecoration: 'underline', cursor: 'pointer', fontSize: 13,
}
const editBtnStyle: React.CSSProperties = {
  padding: '5px 14px', border: '1px solid #ccc', borderRadius: 4,
  background: 'none', cursor: 'pointer', fontSize: 13,
}
const stopBtnStyle: React.CSSProperties = {
  padding: '5px 14px', border: '1px solid #fca5a5', borderRadius: 4,
  background: '#fff', cursor: 'pointer', fontSize: 13, color: '#b91c1c',
}
const runBtnStyle: React.CSSProperties = {
  padding: '5px 14px', border: '1px solid #86efac', borderRadius: 4,
  background: '#fff', cursor: 'pointer', fontSize: 13, color: '#15803d',
}
const logPanelStyle: React.CSSProperties = {
  background: '#1e1e1e', color: '#d4d4d4', padding: 12, borderRadius: 4,
  fontSize: 12, lineHeight: 1.5, overflow: 'auto', maxHeight: 400,
  margin: 0, fontFamily: 'monospace',
}
