import { useEffect, useMemo, useState } from 'react'
import { listCatalog, getPipeline } from './api'
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
      <div style={{ display: 'flex', alignItems: 'center', gap: 16, marginBottom: 24 }}>
        <h1 style={{ fontSize: 22, margin: 0 }}>RPC Operator</h1>
        <div style={{ display: 'flex', gap: 8 }}>
          <button
            onClick={() => setView('list')}
            style={{ ...tabStyle, ...(view === 'list' ? tabActiveStyle : {}) }}
          >
            Pipelines
          </button>
          <button
            onClick={() => setView('editor')}
            style={{ ...tabStyle, ...(view === 'editor' ? tabActiveStyle : {}) }}
          >
            Editor
          </button>
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
          <div style={{ marginBottom: 16 }}>
            <label style={{ fontSize: 14 }}>
              Pipeline-Name&nbsp;
              <input value={name} onChange={e => setName(e.target.value)} style={inputStyle} />
            </label>
          </div>
          <PipelineEditor spec={spec} catalogCache={catalogCache} onChange={setSpec} />
          <DeployBar namespace={namespace} name={name} spec={spec} />
        </>
      )}
    </div>
  )
}

const tabStyle: React.CSSProperties = {
  padding: '4px 14px', border: '1px solid #ccc', borderRadius: 4,
  background: 'none', cursor: 'pointer', fontSize: 14,
}
const tabActiveStyle: React.CSSProperties = {
  background: '#3b82f6', color: '#fff', borderColor: '#3b82f6',
}
const nsInputStyle: React.CSSProperties = {
  padding: '3px 8px', border: '1px solid #ccc', borderRadius: 4, fontSize: 13,
}
const inputStyle: React.CSSProperties = {
  padding: '5px 10px', border: '1px solid #ccc', borderRadius: 4, fontSize: 14,
  marginLeft: 4,
}
