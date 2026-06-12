import type { ConnState, ConnectionsResponse } from '../types'

interface Props {
  /** Live connection state; undefined treated as all-unknown. */
  state: ConnectionsResponse | undefined
}

function dot(cs: ConnState | undefined): React.CSSProperties {
  return {
    display: 'inline-block',
    width: 10,
    height: 10,
    borderRadius: '50%',
    marginRight: 6,
    flexShrink: 0,
    background: cs === 'up' ? '#16a34a' : cs === 'down' ? '#dc2626' : '#94a3b8',
  }
}

function label(cs: ConnState | undefined): string {
  if (cs === 'up') return 'connected'
  if (cs === 'down') return 'disconnected'
  return 'unknown'
}

export function ConnectionLights({ state }: Props) {
  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', marginBottom: 4 }}>
        <span style={dot(state?.input)} />
        <span style={{ fontSize: 12, color: '#555' }}>Input: {label(state?.input)}</span>
      </div>
      <div style={{ display: 'flex', alignItems: 'center' }}>
        <span style={dot(state?.output)} />
        <span style={{ fontSize: 12, color: '#555' }}>Output: {label(state?.output)}</span>
      </div>
    </div>
  )
}
