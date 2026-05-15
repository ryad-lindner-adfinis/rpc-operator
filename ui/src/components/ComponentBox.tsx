import { useState } from 'react'
import { ChevronDown, ChevronRight, X } from 'lucide-react'
import { ComponentPicker } from './ComponentPicker'
import { SchemaForm } from './SchemaForm'
import type { CatalogComponent, ComponentSpec } from '../types'

interface Props {
  title: string
  category: 'inputs' | 'processors' | 'outputs'
  multi?: boolean
  items: ComponentSpec[]
  catalogCache: Map<string, CatalogComponent>
  onChange: (items: ComponentSpec[]) => void
}

export function ComponentBox({ title, category, multi, items, catalogCache, onChange }: Props) {
  const [picking, setPicking] = useState(false)
  const [collapsed, setCollapsed] = useState<boolean[]>([])

  function handleSelect(comp: CatalogComponent) {
    const isDirectArray =
      comp.bodyKind === 'composite' &&
      comp.compositeFields?.length === 1 &&
      comp.compositeFields[0].field === ''

    const newSpec: ComponentSpec = {
      type: comp.name,
      config: comp.bodyKind === 'scalar' ? '' : isDirectArray ? [] : {},
    }
    onChange(multi ? [...items, newSpec] : [newSpec])
    setCollapsed(prev => multi ? [...prev, false] : [false])
    setPicking(false)
  }

  function handleConfigChange(idx: number, config: unknown) {
    onChange(items.map((it, i) => (i === idx ? { ...it, config } : it)))
  }

  function handleLabelChange(idx: number, label: string) {
    onChange(items.map((it, i) => (i === idx ? { ...it, label } : it)))
  }

  function handleRemove(idx: number) {
    onChange(items.filter((_, i) => i !== idx))
    setCollapsed(prev => prev.filter((_, i) => i !== idx))
  }

  function toggleCollapse(idx: number) {
    setCollapsed(prev => {
      const next = [...prev]
      next[idx] = !next[idx]
      return next
    })
  }

  const isCollapsed = (idx: number) => collapsed[idx] ?? false

  return (
    <div style={boxStyle}>
      <h4 style={{ margin: '0 0 12px', color: '#333' }}>{title}</h4>
      {items.map((item, idx) => {
        const comp = catalogCache.get(category + '/' + item.type)
        const open = !isCollapsed(idx)
        return (
          <div key={idx} style={slotStyle}>
            {/* Header row */}
            <div style={headerRowStyle}>
              <button
                onClick={() => toggleCollapse(idx)}
                style={chevronBtnStyle}
                title={open ? 'Einklappen' : 'Ausklappen'}
              >
                {open ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
              </button>

              {multi ? (
                /* Processors: editable label + type below */
                <div style={{ flex: 1, minWidth: 0 }}>
                  <input
                    value={item.label ?? ''}
                    onChange={e => handleLabelChange(idx, e.target.value)}
                    placeholder="Label (Pflichtfeld)"
                    style={labelInputStyle}
                  />
                  <div style={{ fontSize: 11, color: '#888', marginTop: 2 }}>{item.type}</div>
                </div>
              ) : (
                /* Input / Output: type name + composite badge */
                <div style={{ flex: 1, display: 'flex', alignItems: 'center', gap: 8 }}>
                  <strong style={{ fontSize: 14 }}>{item.type}</strong>
                  {comp?.bodyKind === 'composite' && (
                    <span style={compositeBadgeStyle}>composite</span>
                  )}
                </div>
              )}

              <div style={{ display: 'flex', gap: 6, flexShrink: 0 }}>
                {!multi && (
                  <button onClick={() => setPicking(true)} style={changeBtnStyle}>
                    Ändern
                  </button>
                )}
                <button onClick={() => handleRemove(idx)} style={removeBtnStyle}>
                  <X size={14} />
                </button>
              </div>
            </div>

            {/* Body: schema form, only when expanded */}
            {open && comp && (
              <div style={{ marginTop: 10 }}>
                <SchemaForm
                  component={comp}
                  value={item.config}
                  catalogCache={catalogCache}
                  depth={0}
                  onChange={val => handleConfigChange(idx, val)}
                />
              </div>
            )}
          </div>
        )
      })}
      {(multi || items.length === 0) && (
        <button onClick={() => setPicking(true)} style={addBtnStyle}>
          + hinzufügen
        </button>
      )}
      {picking && (
        <ComponentPicker
          category={category}
          onSelect={handleSelect}
          onClose={() => setPicking(false)}
        />
      )}
    </div>
  )
}

const boxStyle: React.CSSProperties = {
  flex: 1,
  border: '2px solid #dde',
  borderRadius: 8,
  padding: 16,
  minWidth: 260,
}
const slotStyle: React.CSSProperties = {
  background: '#f8f9ff',
  borderRadius: 4,
  padding: 12,
  marginBottom: 8,
}
const headerRowStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'flex-start',
  gap: 8,
}
const chevronBtnStyle: React.CSSProperties = {
  background: 'none',
  border: 'none',
  cursor: 'pointer',
  fontSize: 12,
  color: '#666',
  padding: '2px 4px',
  flexShrink: 0,
  lineHeight: 1,
  marginTop: 3,
}
const labelInputStyle: React.CSSProperties = {
  width: '100%',
  fontSize: 13,
  fontWeight: 600,
  padding: '3px 6px',
  border: '1px solid #ccd',
  borderRadius: 3,
  background: '#fff',
  boxSizing: 'border-box',
}
const compositeBadgeStyle: React.CSSProperties = {
  fontSize: 11,
  color: '#3b5',
  background: '#e8fff0',
  padding: '1px 5px',
  borderRadius: 8,
}
const changeBtnStyle: React.CSSProperties = {
  color: '#555',
  border: '1px solid #ccc',
  background: 'none',
  cursor: 'pointer',
  borderRadius: 3,
  fontSize: 12,
  padding: '1px 8px',
}
const removeBtnStyle: React.CSSProperties = {
  color: '#c00',
  border: 'none',
  background: 'none',
  cursor: 'pointer',
}
const addBtnStyle: React.CSSProperties = {
  marginTop: 8,
  padding: '6px 14px',
  cursor: 'pointer',
  borderRadius: 4,
  border: '1px dashed #aab',
  background: 'none',
  width: '100%',
}
