import { useEffect, useRef, useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { toast } from 'sonner'
import { ChevronDown, ChevronRight, ChevronUp, ChevronsUpDown, Pencil, X } from 'lucide-react'
import { listPipelines, deletePipeline } from '../api'
import type { Pipeline } from '../types'

interface Props {
  namespace: string
  /** F42 Mode C: hides Neue Pipeline / Bearbeiten / Loeschen actions. */
  readOnly?: boolean
  onEdit?: (pipeline: Pipeline) => void
  onViewDetail: (pipeline: Pipeline) => void
  onNew?: () => void
  onNewRaw?: () => void
}

type SortKey = 'name' | 'phase' | 'pod' | 'updated'
type SortDir = 'asc' | 'desc'

const phaseOrder: Record<string, number> = { Running: 0, Pending: 1, Failed: 2, Stopped: 3 }

function rawUpdated(p: Pipeline): string {
  const times = (p.status?.conditions ?? []).map(c => c.lastTransitionTime).filter(Boolean) as string[]
  return times.length > 0 ? times.reduce((a, b) => (a > b ? a : b)) : (p.metadata.creationTimestamp ?? '')
}

function sortPipelines(items: Pipeline[], key: SortKey, dir: SortDir): Pipeline[] {
  return [...items].sort((a, b) => {
    let cmp = 0
    if (key === 'name')    cmp = a.metadata.name.localeCompare(b.metadata.name)
    if (key === 'phase')   cmp = (phaseOrder[a.status?.phase ?? ''] ?? 9) - (phaseOrder[b.status?.phase ?? ''] ?? 9)
    if (key === 'pod')     cmp = (a.status?.podName ?? '').localeCompare(b.status?.podName ?? '')
    if (key === 'updated') cmp = rawUpdated(a).localeCompare(rawUpdated(b))
    return dir === 'asc' ? cmp : -cmp
  })
}

export function PipelineList({ namespace, readOnly = false, onEdit, onViewDetail, onNew, onNewRaw }: Props) {
  const [pipelines, setPipelines] = useState<Pipeline[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string>()
  const [dropdownOpen, setDropdownOpen] = useState(false)
  const dropdownRef = useRef<HTMLDivElement>(null)
  const [sortKey, setSortKey] = useState<SortKey>('name')
  const [sortDir, setSortDir] = useState<SortDir>('asc')
  const [deletingNames, setDeletingNames] = useState<Set<string>>(new Set())
  const [pipelineToDelete, setPipelineToDelete] = useState<Pipeline | null>(null)

  function handleSort(key: SortKey) {
    if (key === sortKey) setSortDir(d => d === 'asc' ? 'desc' : 'asc')
    else { setSortKey(key); setSortDir('asc') }
  }

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

  useEffect(() => {
    if (!dropdownOpen) return
    function handleClick(e: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setDropdownOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [dropdownOpen])

  function requestDelete(p: Pipeline, e: React.MouseEvent) {
    e.stopPropagation()
    setPipelineToDelete(p)
  }

  async function confirmDelete() {
    if (!pipelineToDelete) return
    const p = pipelineToDelete
    setPipelineToDelete(null)
    setDeletingNames(prev => new Set(prev).add(p.metadata.name))
    try {
      await deletePipeline(p.metadata.namespace, p.metadata.name)
      toast.success(`Pipeline „${p.metadata.name}" gelöscht`)
      load()
    } catch (err) {
      console.error(err)
      toast.error(`Fehler beim Löschen von „${p.metadata.name}"`)
      setDeletingNames(prev => { const s = new Set(prev); s.delete(p.metadata.name); return s })
      load()
    }
  }

  if (loading) return <p style={{ color: '#888' }}>Lade Pipelines…</p>
  if (error)   return <p style={{ color: 'red' }}>Fehler: {error}</p>

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <h2 style={{ margin: 0, fontSize: 18 }}>Pipelines — {namespace}</h2>
        {!readOnly && onNew && (
          <div ref={dropdownRef} style={{ position: 'relative', display: 'inline-flex' }}>
            <button onClick={onNew} style={newBtnStyle}>+ Neue Pipeline</button>
            <button
              onClick={() => setDropdownOpen(o => !o)}
              style={{ ...newBtnStyle, padding: '6px 8px', borderLeft: '1px solid rgba(255,255,255,0.4)', borderRadius: '0 4px 4px 0' }}
              aria-label="Weitere Optionen"
            ><ChevronDown size={14} /></button>
            {dropdownOpen && onNewRaw && (
              <div style={dropdownMenuStyle}>
                <button
                  onClick={() => { setDropdownOpen(false); onNewRaw() }}
                  style={dropdownItemStyle}
                >
                  Neue RAW Pipeline
                </button>
              </div>
            )}
          </div>
        )}
      </div>
      {pipelines.length === 0 ? (
        <p style={{ color: '#888' }}>Keine Pipelines in diesem Namespace.</p>
      ) : (
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14 }}>
          <thead>
            <tr style={{ background: '#f5f5f5', textAlign: 'left' }}>
              <th style={thStyle} onClick={() => handleSort('name')}    title="Sortieren"><SortHeader label="Name"          col="name"    sortKey={sortKey} sortDir={sortDir} /></th>
              <th style={thStyle} onClick={() => handleSort('phase')}   title="Sortieren"><SortHeader label="Status"        col="phase"   sortKey={sortKey} sortDir={sortDir} /></th>
              <th style={thStyle} onClick={() => handleSort('pod')}     title="Sortieren"><SortHeader label="Pod"           col="pod"     sortKey={sortKey} sortDir={sortDir} /></th>
              <th style={thStyle} onClick={() => handleSort('updated')} title="Sortieren"><SortHeader label="Letztes Update" col="updated" sortKey={sortKey} sortDir={sortDir} /></th>
              <th style={thStyle}></th>
            </tr>
          </thead>
          <tbody>
            {sortPipelines(pipelines, sortKey, sortDir).map(p => {
              const isDeleting = deletingNames.has(p.metadata.name)
              return (
                <tr
                  key={p.metadata.name}
                  onClick={isDeleting ? undefined : () => onViewDetail(p)}
                  style={{ cursor: isDeleting ? 'default' : 'pointer', borderBottom: '1px solid #eee', opacity: isDeleting ? 0.6 : 1 }}
                  onMouseEnter={isDeleting ? undefined : e => (e.currentTarget.style.background = '#f9f9ff')}
                  onMouseLeave={isDeleting ? undefined : e => (e.currentTarget.style.background = '')}
                >
                  <td style={tdStyle}><strong>{p.metadata.name}</strong></td>
                  <td style={tdStyle}>
                    <PhaseBadge phase={isDeleting ? 'Deleting' : p.status?.phase} />
                    <ConditionHint conditions={p.status?.conditions} phase={p.status?.phase} />
                  </td>
                  <td style={{ ...tdStyle, color: '#666', fontFamily: 'monospace', fontSize: 12 }}>
                    {p.status?.podName ?? '—'}
                  </td>
                  <td style={{ ...tdStyle, color: '#666', fontSize: 12 }}>
                    {lastUpdated(p)}
                  </td>
                  <td style={{ ...tdStyle, textAlign: 'right', whiteSpace: 'nowrap' }}>
                    {!readOnly && onEdit && (
                      <button
                        onClick={e => { e.stopPropagation(); onEdit(p) }}
                        title="Bearbeiten"
                        disabled={isDeleting}
                        style={{ ...iconBtnStyle, color: isDeleting ? '#ccc' : '#3b82f6' }}
                      >
                        <Pencil size={14} />
                      </button>
                    )}
                    {!readOnly && (
                      <button
                        onClick={e => requestDelete(p, e)}
                        title="Löschen"
                        disabled={isDeleting}
                        style={{ ...iconBtnStyle, marginLeft: 4, color: isDeleting ? '#ccc' : '#ef4444' }}
                      >
                        <X size={14} />
                      </button>
                    )}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      )}

      <Dialog.Root open={!!pipelineToDelete} onOpenChange={open => { if (!open) setPipelineToDelete(null) }}>
        <Dialog.Portal>
          <Dialog.Overlay style={dialogOverlayStyle} />
          <Dialog.Content style={dialogContentStyle}>
            <Dialog.Title style={{ margin: '0 0 8px', fontSize: 16, fontWeight: 600 }}>
              Pipeline löschen
            </Dialog.Title>
            <Dialog.Description style={{ color: '#555', fontSize: 14, margin: '0 0 20px', lineHeight: 1.5 }}>
              Pipeline <strong>„{pipelineToDelete?.metadata.name}"</strong> wirklich löschen?
              Diese Aktion kann nicht rückgängig gemacht werden.
            </Dialog.Description>
            <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
              <Dialog.Close asChild>
                <button style={dialogCancelBtnStyle}>Abbrechen</button>
              </Dialog.Close>
              <button onClick={confirmDelete} style={dialogDeleteBtnStyle}>Löschen</button>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </div>
  )
}

function PhaseBadge({ phase }: { phase?: string }) {
  const colors: Record<string, { bg: string; text: string }> = {
    Running:  { bg: '#dcfce7', text: '#16a34a' },
    Failed:   { bg: '#fee2e2', text: '#dc2626' },
    Pending:  { bg: '#fef9c3', text: '#d97706' },
    Stopped:  { bg: '#f3f4f6', text: '#6b7280' },
    Deleting: { bg: '#ffedd5', text: '#c2410c' },
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

type Condition = NonNullable<NonNullable<Pipeline['status']>['conditions']>[number]

function ConditionHint({ conditions, phase }: { conditions?: Condition[]; phase?: string }) {
  if (phase !== 'Running') return null
  const readyCond = (conditions ?? []).find(c => c.type === 'Ready')
  if (!readyCond || readyCond.status !== 'False' || !readyCond.reason) return null
  return (
    <span style={{ marginLeft: 6, fontSize: 11, color: '#dc2626', fontWeight: 600, whiteSpace: 'nowrap' }}>
      ⚠ {readyCond.reason}
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

function SortHeader({ label, col, sortKey, sortDir }: { label: string; col: SortKey; sortKey: SortKey; sortDir: SortDir }) {
  const active = sortKey === col
  const Icon = active ? (sortDir === 'asc' ? ChevronUp : ChevronDown) : ChevronsUpDown
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4, cursor: 'pointer', userSelect: 'none' }}>
      {label}
      <Icon size={12} color={active ? '#3b82f6' : '#ccc'} />
    </span>
  )
}

const thStyle: React.CSSProperties = { padding: '8px 12px', fontWeight: 600, fontSize: 13, cursor: 'pointer' }
const tdStyle: React.CSSProperties = { padding: '10px 12px', verticalAlign: 'middle' }
const iconBtnStyle: React.CSSProperties = {
  border: 'none', background: 'none', cursor: 'pointer', fontSize: 14, padding: '2px 4px',
  borderRadius: 4, lineHeight: 1, color: '#999',
}
const newBtnStyle: React.CSSProperties = {
  padding: '6px 16px', background: '#3b82f6', color: '#fff',
  border: 'none', borderRadius: '4px 0 0 4px', cursor: 'pointer', fontSize: 14,
}
const dropdownMenuStyle: React.CSSProperties = {
  position: 'absolute', top: '100%', right: 0, marginTop: 2,
  background: '#fff', border: '1px solid #d1d5db', borderRadius: 4,
  boxShadow: '0 4px 12px rgba(0,0,0,0.12)', zIndex: 100, minWidth: 180,
}
const dropdownItemStyle: React.CSSProperties = {
  display: 'block', width: '100%', padding: '8px 16px', textAlign: 'left',
  background: 'none', border: 'none', cursor: 'pointer', fontSize: 14, color: '#111',
  whiteSpace: 'nowrap',
}
const dialogOverlayStyle: React.CSSProperties = {
  position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', zIndex: 200,
}
const dialogContentStyle: React.CSSProperties = {
  position: 'fixed', top: '50%', left: '50%', transform: 'translate(-50%,-50%)',
  background: '#fff', borderRadius: 8, padding: 24, width: 400, maxWidth: '90vw',
  boxShadow: '0 8px 32px rgba(0,0,0,0.18)', zIndex: 201,
}
const dialogCancelBtnStyle: React.CSSProperties = {
  padding: '7px 16px', border: '1px solid #ccc', borderRadius: 4,
  background: 'none', cursor: 'pointer', fontSize: 14,
}
const dialogDeleteBtnStyle: React.CSSProperties = {
  padding: '7px 16px', border: 'none', borderRadius: 4,
  background: '#ef4444', color: '#fff', cursor: 'pointer', fontSize: 14, fontWeight: 600,
}
