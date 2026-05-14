// Mirrors api/v1alpha1/pipeline_types.go and internal/api/catalog/catalog.go

export interface ComponentSpec {
  type: string
  label?: string
  // config is unknown: string (scalar), object (object/composite Pattern A),
  // or ComponentSpec[] directly (composite Pattern B: for_each, fallback)
  config?: unknown
}

export interface PipelineSpec {
  input?: ComponentSpec
  processors?: ComponentSpec[]
  output?: ComponentSpec
  replicas?: number
  image?: string
}

export interface Pipeline {
  apiVersion: string
  kind: string
  metadata: {
    name: string
    namespace: string
    resourceVersion?: string
    creationTimestamp?: string
  }
  spec: PipelineSpec
  status?: {
    phase?: 'Pending' | 'Running' | 'Failed' | 'Stopped'
    podName?: string
    observedGeneration?: number
    conditions?: Array<{
      type: string
      status: string
      message?: string
      reason?: string
      lastTransitionTime?: string
    }>
  }
}

// Mirrors catalog.CompositeField
export interface CompositeField {
  field: string        // field name in config; "" = config itself is the array (Pattern B)
  kind: 'inputs' | 'processors' | 'outputs'
  multi: boolean
}

export interface CatalogComponent {
  name: string
  category: 'inputs' | 'processors' | 'outputs'
  status: string
  summary: string
  bodyKind: 'object' | 'scalar' | 'composite'
  replicaSafety: string
  configSchema: object            // JSON Schema Draft-07 (non-composite fields only)
  compositeFields?: CompositeField[]
}

export interface ValidationError {
  path: string
  message: string
}

export interface ValidateResponse {
  valid: boolean
  errors?: ValidationError[]
}

export interface MetricsDatapoint {
  t: number  // Unix timestamp (seconds)
  v: number  // value (msg/s)
}

export interface MetricsResponse {
  query: string
  unit: string
  datapoints: MetricsDatapoint[]
}
