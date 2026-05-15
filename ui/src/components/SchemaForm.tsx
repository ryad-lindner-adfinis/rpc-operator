import { useState } from 'react'
import { X } from 'lucide-react'
import Form from '@rjsf/core'
import validator from '@rjsf/validator-ajv8'
import type { IconButtonProps, ArrayFieldTemplateItemType, FieldTemplateProps, ArrayFieldTemplateProps } from '@rjsf/utils'
import { CompositeForm } from './CompositeForm'
import type { CatalogComponent } from '../types'

function AddButton({ onClick }: IconButtonProps) {
  return (
    <button
      onClick={onClick}
      style={{
        marginTop: 8, padding: '6px 14px', cursor: 'pointer',
        borderRadius: 4, border: '1px dashed #aab', background: 'none', width: '100%',
      }}
    >
      + hinzufügen
    </button>
  )
}

function NullButton() { return null }

function infoBtn(open: boolean, toggle: () => void): React.ReactElement {
  return (
    <button
      type="button"
      onClick={toggle}
      title={open ? 'Info ausblenden' : 'Info anzeigen'}
      style={{
        width: 16, height: 16, borderRadius: '50%', flexShrink: 0,
        border: '1px solid #aab', background: open ? '#e8eeff' : 'none',
        color: '#5566aa', cursor: 'pointer', fontSize: 11, fontWeight: 700,
        lineHeight: '14px', padding: 0, textAlign: 'center',
      }}
    >
      i
    </button>
  )
}

function FieldTemplate({ id, label, required, displayLabel, rawDescription, children, errors }: FieldTemplateProps) {
  const [open, setOpen] = useState(false)
  return (
    <div style={{ marginBottom: 8 }}>
      {displayLabel && label && (
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 3 }}>
          <label htmlFor={id} style={{ fontSize: 13, fontWeight: 500 }}>
            {label}{required ? ' *' : ''}
          </label>
          {rawDescription && infoBtn(open, () => setOpen(o => !o))}
        </div>
      )}
      {open && rawDescription && (
        <p style={{ fontSize: 12, color: '#555', margin: '0 0 6px', lineHeight: 1.4 }}>{rawDescription}</p>
      )}
      {children}
      {errors}
    </div>
  )
}

function ArrayFieldTemplate({ items, canAdd, onAddClick, schema, title, uiSchema, registry }: ArrayFieldTemplateProps) {
  const [open, setOpen] = useState(false)
  const description = schema.description
  const { ArrayFieldItemTemplate, ButtonTemplates: { AddButton } } = registry.templates
  return (
    <div>
      {title && (
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 3 }}>
          <span style={{ fontSize: 13, fontWeight: 500 }}>{title}</span>
          {description && infoBtn(open, () => setOpen(o => !o))}
        </div>
      )}
      {open && description && (
        <p style={{ fontSize: 12, color: '#555', margin: '0 0 6px', lineHeight: 1.4 }}>{description}</p>
      )}
      {items.map(({ key, ...itemProps }) => <ArrayFieldItemTemplate key={key} {...itemProps} />)}
      {canAdd && <AddButton onClick={onAddClick} registry={registry} uiSchema={uiSchema} />}
    </div>
  )
}

const ArrayItemTemplate = ({ children, hasRemove, onDropIndexClick, index }: ArrayFieldTemplateItemType) => (
  <div style={{ display: 'flex', alignItems: 'center', gap: 4, marginBottom: 4 }}>
    <div style={{ flex: 1 }}>{children}</div>
    {hasRemove && (
      <button
        onClick={onDropIndexClick(index)}
        style={{ color: '#c00', border: 'none', background: 'none', cursor: 'pointer', fontSize: 14, flexShrink: 0, display: 'flex' }}
      >
        <X size={14} />
      </button>
    )}
  </div>
)

const formTemplates = {
  FieldTemplate,
  ArrayFieldTemplate,
  ArrayFieldItemTemplate: ArrayItemTemplate,
  ButtonTemplates: { AddButton, MoveUpButton: NullButton, MoveDownButton: NullButton },
}

interface Props {
  component: CatalogComponent
  value: unknown
  catalogCache: Map<string, CatalogComponent>
  depth?: number
  onChange: (value: unknown) => void
}

// SchemaForm is the central renderer for all bodyKind variants.
// It passes itself as SchemaFormComp to CompositeForm → NestedComponentEditor,
// enabling full recursive rendering of nested composite components.
export function SchemaForm({ component, value, catalogCache, depth = 0, onChange }: Props) {
  if (component.bodyKind === 'scalar') {
    return (
      <textarea
        value={typeof value === 'string' ? value : ''}
        onChange={e => onChange(e.target.value)}
        rows={3}
        style={{
          width: '100%',
          fontFamily: 'monospace',
          fontSize: 12,
          boxSizing: 'border-box',
        }}
        placeholder={`${component.name} expression…`}
      />
    )
  }

  if (component.bodyKind === 'composite') {
    return (
      <CompositeForm
        component={component}
        value={value}
        catalogCache={catalogCache}
        depth={depth}
        SchemaFormComp={SchemaForm}
        onChange={onChange}
      />
    )
  }

  // bodyKind === 'object' (default)
  return (
    <Form
      schema={component.configSchema as object}
      validator={validator}
      formData={value ?? {}}
      onChange={({ formData }) => onChange(formData)}
      uiSchema={{ 'ui:submitButtonOptions': { norender: true } }}
      templates={formTemplates}
    />
  )
}
