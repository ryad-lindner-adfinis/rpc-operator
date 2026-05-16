import { useEffect, useMemo, useState } from 'react'
import { Toaster } from 'sonner'
import { listCatalog, getPipeline } from './api'
import benthosLogo from './assets/benthos-logo.svg'
import { PipelineEditor } from './components/PipelineEditor'
import { PipelineList } from './components/PipelineList'
import { PipelineDetail } from './components/PipelineDetail'
import { RawPipelineEditor } from './components/RawPipelineEditor'
import { DeployBar } from './components/DeployBar'
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

  useEffect(() => { listCatalog().then(setCatalog).catch(console.error) }, [])
  const catalogCache = useMemo(
    () => new Map(catalog.map(c => [c.category + '/' + c.name, c])),
    [catalog],
  )

  async function handleEdit(pipeline: Pipeline) {
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

  function handleNew() {
    setName('my-pipeline')
    setSpec(DEFAULT_SPEC)
    setEditPipeline(undefined)
    setView('editor')
  }

  function handleNewRaw() {
    setEditPipeline(undefined)
    setView('raw-editor')
  }

  return (
    <div style={{ maxWidth: 1200, margin: '0 auto', padding: 24, fontFamily: 'system-ui, sans-serif' }}>
      <Toaster position="bottom-right" richColors />
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
        <div style={{ marginLeft: 'auto' }}>
          <label style={{ fontSize: 13, color: '#555' }}>
            Namespace&nbsp;
            <input
              value={namespace}
              onChange={e => setNamespace(e.target.value)}
              style={nsInputStyle}
            />
          </label>
        </div>
      </div>

      {view === 'list' && (
        <PipelineList
          namespace={namespace}
          onEdit={handleEdit}
          onViewDetail={handleViewDetail}
          onNew={handleNew}
          onNewRaw={handleNewRaw}
        />
      )}

      {view === 'detail' && selectedPipeline && (
        <PipelineDetail
          pipeline={selectedPipeline}
          onEdit={() => handleEdit(selectedPipeline)}
          onBack={() => setView('list')}
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
            <button onClick={() => setView('list')} style={backLinkStyle}>← Zurück</button>
            <label style={{ fontSize: 14 }}>
              Pipeline-Name&nbsp;
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
