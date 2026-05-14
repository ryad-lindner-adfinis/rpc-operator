import Form from '@rjsf/core'
import validator from '@rjsf/validator-ajv8'
import type { IconButtonProps, ArrayFieldTemplateItemType } from '@rjsf/utils'
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

const ArrayItemTemplate = ({ children, hasRemove, onDropIndexClick, index }: ArrayFieldTemplateItemType) => (
  <div style={{ display: 'flex', alignItems: 'center', gap: 4, marginBottom: 4 }}>
    <div style={{ flex: 1 }}>{children}</div>
    {hasRemove && (
      <button
        onClick={onDropIndexClick(index)}
        style={{ color: '#c00', border: 'none', background: 'none', cursor: 'pointer', fontSize: 14, flexShrink: 0 }}
      >
        ✕
      </button>
    )}
  </div>
)

const formTemplates = {
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
