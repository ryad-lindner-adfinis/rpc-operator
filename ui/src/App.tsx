import { useEffect, useMemo, useRef, useState } from 'react'
import { Toaster, toast } from 'sonner'
import {
  listCatalog, getPipeline, listNamespaces, whoami, authConfig,
  stopPipeline, runPipeline, refreshOIDC, oidcLogout, renderPipelineYAML,
  createProject,
  type WhoamiResponse,
} from './api'
import { clearToken, setToken } from './auth'
import benthosLogo from './assets/benthos-logo.svg'
import { PipelineEditor } from './components/PipelineEditor'
import { PipelineList } from './components/PipelineList'
import { PipelineDetail } from './components/PipelineDetail'
import { RawPipelineEditor } from './components/RawPipelineEditor'
import { DeployBar } from './components/DeployBar'
import { LoginScreen } from './components/LoginScreen'
import { Sidebar, type Section } from './components/Sidebar'
import { ClusterList } from './components/ClusterList'
import { ClusterDetail } from './components/ClusterDetail'
import { ProjectList } from './components/ProjectList'
import { ProjectDetail } from './components/ProjectDetail'
import { ProjectForm } from './components/ProjectForm'
import type { CatalogComponent, Pipeline, PipelineSpec, PipelineProjectSpec, ProjectRoute, ProjectCacheResource } from './types'
import { pipelineBackTarget, editorBackTarget, type PipelineOrigin, type EditorOrigin } from './pipelineNav'

const DEFAULT_SPEC: PipelineSpec = {
  input: { type: 'generate', config: { mapping: 'root = "hello world"', interval: '1s', count: 5 } },
  processors: [{ type: 'mapping', config: 'root = content().uppercase()' }],
  output: { type: 'stdout', config: {} },
}

type View = 'list' | 'editor' | 'raw-editor' | 'detail'

export default function App() {
  const [view, setView] = useState<View>('list')
  const [section, setSection] = useState<Section>('pipelines')
  const [clustersView, setClustersView] = useState<'list' | 'detail'>('list')
  const [selectedClusterName, setSelectedClusterName] = useState<string>('')
  const [projectsView, setProjectsView] = useState<'list' | 'detail'>('list')
  const [selectedProjectName, setSelectedProjectName] = useState<string>('')
  const [showProjectForm, setShowProjectForm] = useState(false)
  const [pipelineOrigin, setPipelineOrigin] = useState<PipelineOrigin>({ kind: 'pipelines' })
  const [editorOrigin, setEditorOrigin] = useState<EditorOrigin>({ kind: 'list' })
  const [newPipelineProjectRef, setNewPipelineProjectRef] = useState('')
  const [projectDraftRoutes, setProjectDraftRoutes] = useState<ProjectRoute[]>([])
  const [projectDraftCaches, setProjectDraftCaches] = useState<ProjectCacheResource[]>([])
  const [projectDirty, setProjectDirty] = useState(false)
  const [namespace, setNamespace] = useState('rpc-operator-poc')
  const [name, setName] = useState('my-pipeline')
  const [spec, setSpec] = useState<PipelineSpec>(DEFAULT_SPEC)
  const [catalog, setCatalog] = useState<CatalogComponent[]>([])
  const [selectedPipeline, setSelectedPipeline] = useState<Pipeline | null>(null)
  const [editPipeline, setEditPipeline] = useState<Pipeline | undefined>(undefined)
  const [allowedNamespaces, setAllowedNamespaces] = useState<string[]>([])
  const [me, setMe] = useState<WhoamiResponse | null>(null)
  const [authReady, setAuthReady] = useState(false)
  // F44: when true, render LoginScreen on top of the current state.
  // Used in Mode C when the user clicks the "Log in" banner button.
  const [loginOverlay, setLoginOverlay] = useState(false)
  // F20b: OIDC availability from the token-free /auth/config probe. Needed
  // pre-login because whoami 401s in Mode B strict. The ref mirrors it so the
  // once-registered onExpire listener reads the live value, not a stale closure.
  const [oidcEnabled, setOidcEnabled] = useState(false)
  const oidcEnabledRef = useRef(false)
  const [visualEditorEnabled, setVisualEditorEnabled] = useState(false)

  useEffect(() => {
    // F20b: when the OIDC callback redirected us back with #id_token=... in the
    // URL fragment, pick it up before the first whoami so the very first call
    // is already authenticated. history.replaceState scrubs the fragment so a
    // reload or share-link does not leak the token.
    if (window.location.hash.startsWith('#id_token=')) {
      const t = decodeURIComponent(window.location.hash.slice('#id_token='.length))
      if (t) setToken(t)
      history.replaceState(null, '', window.location.pathname + window.location.search)
    }

    // F20b: token-free probe so the SSO button can render before login (whoami
    // 401s in Mode B strict). Mirror into the ref for the onExpire closure.
    authConfig()
      .then(c => {
        setOidcEnabled(c.oidcEnabled)
        oidcEnabledRef.current = c.oidcEnabled
        setVisualEditorEnabled(c.visualEditorEnabled)
      })
      .catch(() => { /* probe failure → no SSO button, token-paste still works */ })

    whoami()
      .then(r => { setMe(r); setAuthReady(true) })
      .catch(() => { setMe(null); setAuthReady(true) })
    // F44 + F20b: on auth-expire (server 401 after a stored token), re-resolve.
    // When OIDC is enabled, try a silent refresh first — Mode B users keep
    // working without an IdP roundtrip. On any refresh failure, fall back to
    // whoami (Mode C stays anonymous; Mode B strict drops to LoginScreen).
    const onExpire = async () => {
      if (oidcEnabledRef.current) {
        try {
          const newToken = await refreshOIDC()
          setToken(newToken)
          const r = await whoami()
          setMe(r)
          return
        } catch {
          // fall through to plain whoami refresh
        }
      }
      whoami().then(setMe).catch(() => setMe(null))
    }
    window.addEventListener('rpc-auth-expired', onExpire)
    return () => window.removeEventListener('rpc-auth-expired', onExpire)
    // onExpire reads OIDC availability via oidcEnabledRef (not a captured value),
    // so the once-registered listener always sees the live flag. Effect runs once.
    // eslint-disable-next-line react-hooks/exhaustive-deps
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

  async function handleEdit(pipeline: Pipeline, origin: EditorOrigin = { kind: 'list' }) {
    if (readOnly) return
    setEditorOrigin(origin)
    setNewPipelineProjectRef('')
    try {
      const loaded = await getPipeline(pipeline.metadata.namespace, pipeline.metadata.name)
      setNamespace(loaded.metadata.namespace)
      if (loaded.spec.rawYAML) {
        setEditPipeline(loaded)
        setView('raw-editor')
      } else if (!visualEditorEnabled) {
        try {
          const yaml = await renderPipelineYAML(loaded.metadata.namespace, loaded.metadata.name, loaded.spec)
          setEditPipeline({ ...loaded, spec: { ...loaded.spec, rawYAML: yaml } })
          setView('raw-editor')
        } catch (e) {
          toast.error('Render failed: ' + (e as Error).message)
        }
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
      } else if (!visualEditorEnabled) {
        try {
          const yaml = await renderPipelineYAML(pipeline.metadata.namespace, pipeline.metadata.name, pipeline.spec)
          setEditPipeline({ ...pipeline, spec: { ...pipeline.spec, rawYAML: yaml } })
          setView('raw-editor')
        } catch (e) {
          toast.error('Render failed: ' + (e as Error).message)
        }
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
    setPipelineOrigin({ kind: 'pipelines' })  // opened from the list → Back returns to the list
    setView('detail')
  }

  async function openPipelineByName(pipelineName: string, origin: PipelineOrigin = { kind: 'pipelines' }) {
    try {
      const loaded = await getPipeline(namespace, pipelineName)
      setSelectedPipeline(loaded)
      setPipelineOrigin(origin)
      setSection('pipelines')
      setView('detail')
    } catch (e) {
      toast.error('Could not open pipeline: ' + (e as Error).message)
    }
  }

  async function editPipelineByName(pipelineName: string, origin: EditorOrigin) {
    try {
      const loaded = await getPipeline(namespace, pipelineName)
      handleEdit(loaded, origin)
    } catch (e) {
      toast.error('Could not open pipeline: ' + (e as Error).message)
    }
  }

  // Route the pipeline-detail Back button to wherever it was opened from.
  function backFromPipelineDetail() {
    const t = pipelineBackTarget(pipelineOrigin)
    setSection(t.section)
    if (t.section === 'projects') setProjectsView(t.projectsView)
    else if (t.section === 'clusters') setClustersView(t.clustersView)
    else setView(t.pipelinesView)
    setPipelineOrigin({ kind: 'pipelines' })
  }

  // Route the editor's Back button to wherever the editor was opened from.
  function backFromEditor() {
    const t = editorBackTarget(editorOrigin)
    if (t.section === 'projects') {
      setSection('projects')
      setProjectsView('detail')
    } else {
      setSection('pipelines')
      setView(t.pipelinesView)
    }
    setEditorOrigin({ kind: 'list' })
  }

  // Route the editor's Save: a detail-origin edit returns to the REFRESHED
  // detail (re-fetched, original pipelineOrigin preserved); a project-origin
  // create returns to the project; otherwise the pipeline list.
  function savedFromEditor() {
    if (editorOrigin.kind === 'detail' && selectedPipeline) {
      openPipelineByName(selectedPipeline.metadata.name, pipelineOrigin)
    } else if (editorOrigin.kind === 'project') {
      setSection('projects')
      setProjectsView('detail')
    } else {
      // detail-origin but selectedPipeline was cleared in the meantime → list.
      setView('list')
    }
    setEditorOrigin({ kind: 'list' })
  }

  function openClusterByName(clusterName: string) {
    setSelectedClusterName(clusterName)
    setSection('clusters')
    setClustersView('detail')
  }

  async function handleCreateProject(projectName: string, projSpec: PipelineProjectSpec) {
    await createProject(namespace, projectName, projSpec)
    setShowProjectForm(false)
    setSelectedProjectName(projectName)
    setProjectsView('detail')
    setProjectDraftRoutes([])
    setProjectDraftCaches([])
    setProjectDirty(false)
    toast.success(`Created project ${projectName}`)
  }

  // Open the pipeline editor with projectRef pre-filled (visual editor reads
  // spec.projectRef; the raw editor reads initialProjectRef). Back/Save return
  // to the project via editorOrigin.
  function handleAddProjectPipeline(projectName: string) {
    if (readOnly) return
    setEditorOrigin({ kind: 'project', name: projectName })
    setNewPipelineProjectRef(projectName)
    if (!visualEditorEnabled) {
      setEditPipeline(undefined)
      setName('my-pipeline')
      setSpec({ projectRef: { name: projectName }, rawYAML: '' })
      setView('raw-editor')
      setSection('pipelines')
      return
    }
    setName('my-pipeline')
    setSpec({ ...DEFAULT_SPEC, projectRef: { name: projectName } })
    setEditPipeline(undefined)
    setView('editor')
    setSection('pipelines')
  }

  // F44: central entry point for "user wants to authenticate".
  // F20b (OIDC) will replace this body with a PKCE redirect — call sites stay the same.
  function triggerLogin() {
    setLoginOverlay(true)
  }

  // F44 + F20b: logout that works in both Mode B and Mode C. When OIDC is on,
  // first drop the backend-side session (best-effort, never blocks the UI).
  // Then clear the local token and re-resolve via whoami: 200 anonymous (Mode A
  // or C) → stay in anonymous view; 401 (Mode B strict) → LoginScreen.
  async function handleLogout() {
    if (me?.oidcEnabled) {
      await oidcLogout()
    }
    clearToken()
    setLoginOverlay(false)
    try {
      const r = await whoami()
      setMe(r)
    } catch {
      setMe(null)
    }
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
    setEditorOrigin({ kind: 'list' })
    setNewPipelineProjectRef('')
    if (!visualEditorEnabled) { handleNewRaw(); return }
    setName('my-pipeline')
    setSpec(DEFAULT_SPEC)
    setEditPipeline(undefined)
    setView('editor')
  }

  function handleNewRaw() {
    if (readOnly) return
    setEditorOrigin({ kind: 'list' })
    setNewPipelineProjectRef('')
    setEditPipeline(undefined)
    setView('raw-editor')
  }

  if (!authReady) {
    return <div style={{ padding: 24, fontFamily: 'system-ui, sans-serif' }}>Loading…</div>
  }
  if (!me || loginOverlay) {
    // F44: Cancel is only meaningful in Mode C — Mode B users without a token
    // have no working state to cancel back to.
    const cancelLogin =
      me && me.anonymous && me.readOnly
        ? () => setLoginOverlay(false)
        : undefined
    return (
      <>
        <Toaster position="bottom-right" richColors />
        <LoginScreen
          onLoggedIn={() => {
            whoami()
              .then(r => { setMe(r); setLoginOverlay(false) })
              .catch(() => setMe(null))
          }}
          onCancel={cancelLogin}
          oidcEnabled={oidcEnabled}
        />
      </>
    )
  }

  return (
    <div style={{ padding: '24px 32px', fontFamily: 'system-ui, sans-serif' }}>
      <Toaster position="bottom-right" richColors />
      {readOnly && (
        <div style={readOnlyBannerStyle}>
          <span>
            Read-only mode — anonymous access. Editing and deploying are not available.
          </span>
          <button onClick={triggerLogin} style={bannerLoginBtnStyle}>
            Log in
          </button>
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
              <button onClick={handleLogout} style={logoutBtnStyle}>
                Logout
              </button>
            </>
          )}
        </div>
      </div>

      <div style={{ display: 'flex', gap: 24, alignItems: 'flex-start' }}>
        <Sidebar section={section} onSelect={setSection} />
        <div style={{ flex: 1, minWidth: 0 }}>
          {section === 'pipelines' && (
            <>
              {view === 'list' && (
                <PipelineList
                  namespace={namespace}
                  readOnly={readOnly}
                  onEdit={readOnly ? undefined : handleEdit}
                  onViewDetail={handleViewDetail}
                  onNew={readOnly ? undefined : handleNew}
                  onNewRaw={readOnly ? undefined : (visualEditorEnabled ? handleNewRaw : undefined)}
                />
              )}

              {view === 'detail' && selectedPipeline && (
                <PipelineDetail
                  pipeline={selectedPipeline}
                  readOnly={readOnly}
                  showLogs={!me.anonymous || me.anonymousLogs}
                  onEdit={readOnly ? () => {} : () => handleEdit(selectedPipeline, { kind: 'detail' })}
                  onBack={backFromPipelineDetail}
                  onStop={readOnly ? undefined : handleStop}
                  onRun={readOnly ? undefined : handleRun}
                  onOpenCluster={openClusterByName}
                />
              )}

              {view === 'raw-editor' && (
                <RawPipelineEditor
                  namespace={namespace}
                  editPipeline={editPipeline}
                  initialProjectRef={newPipelineProjectRef}
                  onBack={backFromEditor}
                  onSaved={savedFromEditor}
                />
              )}

              {view === 'editor' && (
                <>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 16, marginBottom: 16 }}>
                    <button onClick={backFromEditor} style={backLinkStyle}>← Back</button>
                    <label style={{ fontSize: 14 }}>
                      Pipeline name&nbsp;
                      <input value={name} onChange={e => setName(e.target.value)} style={inputStyle} />
                    </label>
                  </div>
                  <PipelineEditor namespace={namespace} name={name} spec={spec} catalogCache={catalogCache} onChange={setSpec} />
                  <DeployBar namespace={namespace} name={name} spec={spec} />
                </>
              )}
            </>
          )}

          {section === 'clusters' && clustersView === 'list' && (
            <ClusterList
              namespace={namespace}
              onViewDetail={name => { setSelectedClusterName(name); setClustersView('detail') }}
            />
          )}

          {section === 'clusters' && clustersView === 'detail' && (
            <ClusterDetail
              namespace={namespace}
              name={selectedClusterName}
              readOnly={readOnly}
              onBack={() => setClustersView('list')}
              onOpenPipeline={p => openPipelineByName(p, { kind: 'cluster', name: selectedClusterName })}
            />
          )}

          {section === 'projects' && projectsView === 'list' && (
            <ProjectList
              namespace={namespace}
              onViewDetail={name => { setSelectedProjectName(name); setProjectsView('detail'); setProjectDraftRoutes([]); setProjectDraftCaches([]); setProjectDirty(false) }}
              onNew={readOnly ? undefined : () => setShowProjectForm(true)}
            />
          )}

          {section === 'projects' && projectsView === 'detail' && (
            <ProjectDetail
              namespace={namespace}
              name={selectedProjectName}
              readOnly={readOnly}
              onBack={() => setProjectsView('list')}
              onOpenPipeline={p => openPipelineByName(p, { kind: 'project', name: selectedProjectName })}
              onEditPipeline={p => editPipelineByName(p, { kind: 'project', name: selectedProjectName })}
              onAddPipeline={handleAddProjectPipeline}
              draftRoutes={projectDraftRoutes}
              draftCaches={projectDraftCaches}
              dirty={projectDirty}
              setDraftRoutes={setProjectDraftRoutes}
              setDraftCaches={setProjectDraftCaches}
              setDirty={setProjectDirty}
            />
          )}
        </div>
      </div>

      {showProjectForm && (
        <ProjectForm onCreate={handleCreateProject} onClose={() => setShowProjectForm(false)} />
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
  display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12,
}
const bannerLoginBtnStyle: React.CSSProperties = {
  padding: '4px 12px', fontSize: 12, background: '#fff',
  border: '1px solid #fbbf24', borderRadius: 4, cursor: 'pointer',
  color: '#92400e', fontWeight: 500,
}
