import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { getCluster, updateCluster } from '../api'

interface Props {
  namespace: string
  name: string
  /** Current desired replicas, used to seed the input. */
  replicas: number
  /** Called after a successful scale so the parent can refresh. */
  onScaled: () => void
}

export function ClusterScaleControl({ namespace, name, replicas, onScaled }: Props) {
  const [value, setValue] = useState(replicas)
  const [busy, setBusy] = useState(false)

  // Re-seed when the parent reloads with a new desired count.
  useEffect(() => { setValue(replicas) }, [replicas])

  async function apply() {
    if (value < 1) {
      toast.error('Replicas must be at least 1.')
      return
    }
    setBusy(true)
    try {
      // The 3b PUT replaces .spec wholesale — read the full spec, then bump replicas.
      const current = await getCluster(namespace, name)
      await updateCluster(
        namespace,
        name,
        { ...current.spec, replicas: value },
        current.metadata.resourceVersion,
      )
      toast.success(`Scaled ${name} to ${value} replica(s)`)
      onScaled()
    } catch (e) {
      toast.error('Scale failed: ' + (e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
      <label style={{ fontSize: 13, color: '#555' }}>Replicas</label>
      <input
        type="number"
        min={1}
        value={value}
        onChange={e => setValue(Number(e.target.value))}
        style={inputStyle}
      />
      <button onClick={apply} disabled={busy || value === replicas} style={applyBtnStyle}>
        {busy ? 'Applying…' : 'Apply'}
      </button>
    </div>
  )
}

const inputStyle: React.CSSProperties = {
  width: 64, padding: '4px 8px', border: '1px solid #ccc', borderRadius: 4, fontSize: 13,
}
const applyBtnStyle: React.CSSProperties = {
  padding: '5px 14px', border: '1px solid #ccc', borderRadius: 4,
  background: '#fff', cursor: 'pointer', fontSize: 13,
}
