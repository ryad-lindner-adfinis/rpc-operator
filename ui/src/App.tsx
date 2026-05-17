import { useEffect, useMemo, useState } from 'react'
import { Toaster, toast } from 'sonner'
import {
  listCatalog, getPipeline, listNamespaces, whoami,
  stopPipeline, runPipeline, type WhoamiResponse,
} from './api'
import { clearToken } from './auth'
import benthosLogo from './assets/benthos-logo.svg'
import { PipelineEditor } from './components/PipelineEditor'
import { PipelineList } from './components/PipelineList'
import { PipelineDetail } from './components/PipelineDetail'
import { RawPipelineEditor } from './components/RawPipelineEditor'
import { DeployBar } from './components/DeployBar'
import { LoginScreen } from './components/LoginScreen'
import type { CatalogComponent, Pipeline, PipelineSpec } from './types'

const DEFAULT_SPEC: PipelineSpec = {
  input: { type: 'generate', config: { mapping: 'root = "hello world"', interval: '1s', count: 5 } },
  processors: [{ type: 'mapping', config: 'root = content().uppercase()' }],
  output: { type: 'stdout', config: {} },
}

type View = 'list' | 'editor' | 'raw-editor' | 'detail'

export default function App() {
  const [view, setView] = useState<View>('list')
  const [namespace, setNamespace] = useState('rpc-operator-poc')
  const [name, setName] = useState('my-pipeline')
  const [spec, setSpec] = useState<PipelineSpec>(DEFAULT_SPEC)
  const [catalog, setCatalog] = useState<CatalogComponent[]>([])
  const [selectedPipeline, setSelectedPipeline] = useState<Pipeline | null>(null)
  const [editPipeline, setEditPipeline] = useState<Pipeline | undefined>(undefined)
  const [allowedNamespaces, setAllowedNamespaces] = useState<string[]>([])
  const [me, setMe] = useState<WhoamiResponse | null>(null)
  const [authReady, setAuthReady] = useState(false)

  useEffect(() => {
    whoami()
      .then(r => { setMe(r); setAuthReady(true) })
      .catch(() => { setMe(null); setAuthReady(true) })
    const onExpire = () => { setMe(null) }
    window.addEventListener('rpc-auth-expired', onExpire)
    return () => window.removeEventListener('rpc-auth-expired', onExpire)
  }, [])

  useEffect(() => {
    if (!me) return
    listCatalog().then(setCatalog).catch(console.error)
  }, [me])
  useEffect(() => {
    if (!me) return
    listNamespaces()
      .then(ns => {
        setAllowedNamespaces(ns)
        if (ns.length > 0 && !ns.includes(namespace)) {
          setNamespace(ns[0])
        }
      })
      .catch(console.error)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [me])
  const catalogCache = useMemo(
    () => new Map(catalog.map(c => [c.category + '/' + c.name, c])),
    [catalog],
  )

  // F42: defensive — in Mode C (anonymous read-only), refuse to enter any edit/new view.
  // Buttons that trigger these are also hidden, but this guards against accidental calls.
  const readOnly = me?.readOnly ?? false

  async function handleEdit(pipeline: Pipeline) {
    if (readOnly) return
    try {
      const loaded = await getPipeline(pipeline.metadata.namespace, pipeline.metadata.name)
      setNamespace(loaded.metadata.namespace)
      if (loaded.spec.rawYAML) {
        setEditPipeline(loaded)
        setView('raw-editor')
      } else {
        setName(loaded.metadata.name)
        setSpec(loaded.spec)
        setEditPipeline(undefined)
        setView('editor')
      }
    } catch {
      setNamespace(pipeline.metadata.namespace)
      if (pipeline.spec.rawYAML) {
        setEditPipeline(pipeline)
        setView('raw-editor')
      } else {
        setName(pipeline.metadata.name)
        setSpec(pipeline.spec)
        setEditPipeline(undefined)
        setView('editor')
      }
    }
  }

  function handleViewDetail(pipeline: Pipeline) {
    setSelectedPipeline(pipeline)
    setView('detail')
  }

  async function handleStop() {
    if (!selectedPipeline) return
    const { namespace, name } = selectedPipeline.metadata
    try {
      await stopPipeline(namespace, name)
      const updated = await getPipeline(namespace, name)
      setSelectedPipeline(updated)
      toast.success(`Stopped ${name}`)
    } catch (e) {
      toast.error('Stop failed: ' + (e as Error).message)
    }
  }

  async function handleRun() {
    if (!selectedPipeline) return
    const { namespace, name } = selectedPipeline.metadata
    try {
      await runPipeline(namespace, name)
      const updated = await getPipeline(namespace, name)
      setSelectedPipeline(updated)
      toast.success(`Started ${name}`)
    } catch (e) {
      toast.error('Run failed: ' + (e as Error).message)
    }
  }

  function handleNew() {
    if (readOnly) return
    setName('my-pipeline')
    setSpec(DEFAULT_SPEC)
    setEditPipeline(undefined)
    setView('editor')
  }

  function handleNewRaw() {
    if (readOnly) return
    setEditPipeline(undefined)
    setView('raw-editor')
  }

  if (!authReady) {
    return <div style={{ padding: 24, fontFamily: 'system-ui, sans-serif' }}>Loading…</div>
  }
  if (!me) {
    return (
      <>
        <Toaster position="bottom-right" richColors />
        <LoginScreen onLoggedIn={() => whoami().then(setMe).catch(() => setMe(null))} />
      </>
    )
  }

  return (
    <div style={{ maxWidth: 1200, margin: '0 auto', padding: 24, fontFamily: 'system-ui, sans-serif' }}>
      <Toaster position="bottom-right" richColors />
      {readOnly && (
        <div style={readOnlyBannerStyle}>
          Read-only mode — anonymous access. Editing and deploying are not available.
        </div>
      )}
      <div style={{ display: 'flex', alignItems: 'center', gap: 16, marginBottom: 24 }}>
        <img
          src={benthosLogo}
          alt="Benthos"
          style={{ height: 52, width: 'auto', flexShrink: 0 }}
        />
        <div style={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
          <h1 style={{ fontSize: 20, margin: 0, fontWeight: 600, lineHeight: 1.2 }}>Redpanda Connect Operator</h1>
          <span style={{ fontSize: 12, color: '#aaa', lineHeight: 1 }}>aka Benthos</span>
        </div>
        <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 12 }}>
          <label style={{ fontSize: 13, color: '#555' }}>
            Namespace&nbsp;
            {allowedNamespaces.length > 0 ? (
              <select
                value={namespace}
                onChange={e => setNamespace(e.target.value)}
                style={nsInputStyle}
              >
                {allowedNamespaces.map(ns => (
                  <option key={ns} value={ns}>{ns}</option>
                ))}
              </select>
            ) : (
              <input
                value={namespace}
                onChange={e => setNamespace(e.target.value)}
                style={nsInputStyle}
              />
            )}
          </label>
          {me.anonymous && me.readOnly ? (
            // F42 Mode C — banner above content carries the messaging; header stays clean.
            null
          ) : me.anonymous ? (
            // F43 Mode A — auth disabled entirely.
            <span title="Operator deployed without authentication" style={authHintStyle}>
              Auth disabled
            </span>
          ) : (
            // F20a Mode B — authenticated user.
            <>
              <span style={{ fontSize: 12, color: '#666' }}>{me.user.name}</span>
              <button
                onClick={() => { clearToken(); setMe(null) }}
                style={logoutBtnStyle}
              >
                Logout
              </button>
            </>
          )}
        </div>
      </div>

      {view === 'list' && (
        <PipelineList
          namespace={namespace}
          readOnly={readOnly}
          onEdit={readOnly ? undefined : handleEdit}
          onViewDetail={handleViewDetail}
          onNew={readOnly ? undefined : handleNew}
          onNewRaw={readOnly ? undefined : handleNewRaw}
        />
      )}

      {view === 'detail' && selectedPipeline && (
        <PipelineDetail
          pipeline={selectedPipeline}
          readOnly={readOnly}
          showLogs={!me.anonymous || me.anonymousLogs}
          onEdit={readOnly ? () => {} : () => handleEdit(selectedPipeline)}
          onBack={() => setView('list')}
          onStop={readOnly ? undefined : handleStop}
          onRun={readOnly ? undefined : handleRun}
        />
      )}

      {view === 'raw-editor' && (
        <RawPipelineEditor
          namespace={namespace}
          editPipeline={editPipeline}
          onBack={() => setView('list')}
          onSaved={() => setView('list')}
        />
      )}

      {view === 'editor' && (
        <>
          <div style={{ display: 'flex', alignItems: 'center', gap: 16, marginBottom: 16 }}>
            <button onClick={() => setView('list')} style={backLinkStyle}>← Back</button>
            <label style={{ fontSize: 14 }}>
              Pipeline name&nbsp;
              <input value={name} onChange={e => setName(e.target.value)} style={inputStyle} />
            </label>
          </div>
          <PipelineEditor namespace={namespace} name={name} spec={spec} catalogCache={catalogCache} onChange={setSpec} />
          <DeployBar namespace={namespace} name={name} spec={spec} />
        </>
      )}
    </div>
  )
}

const backLinkStyle: React.CSSProperties = {
  border: 'none', background: 'none', cursor: 'pointer', fontSize: 14, color: '#3b82f6',
}
const nsInputStyle: React.CSSProperties = {
  padding: '3px 8px', border: '1px solid #ccc', borderRadius: 4, fontSize: 13,
}
const inputStyle: React.CSSProperties = {
  padding: '5px 10px', border: '1px solid #ccc', borderRadius: 4, fontSize: 14,
  marginLeft: 4,
}
const authHintStyle: React.CSSProperties = {
  fontSize: 11, color: '#999', fontStyle: 'italic', cursor: 'help',
}
const logoutBtnStyle: React.CSSProperties = {
  padding: '4px 10px', fontSize: 12, background: '#fff', border: '1px solid #ccc',
  borderRadius: 4, cursor: 'pointer', color: '#444',
}
const readOnlyBannerStyle: React.CSSProperties = {
  background: '#fef3c7', border: '1px solid #fbbf24', borderRadius: 4,
  padding: '8px 12px', marginBottom: 16, fontSize: 13, color: '#92400e',
}
