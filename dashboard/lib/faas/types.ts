/** Lifecycle status for a demo container / Railway deployment */
export type FaasContainerStatus =
  | 'idle'
  | 'creating'
  | 'deploying'
  | 'running'
  | 'failed'
  | 'stopping'
  | 'stopped'

export type FaasLogLevel = 'info' | 'warn' | 'error'

export interface FaasLogLine {
  ts: string
  level: FaasLogLevel
  message: string
}

/** Curated template shown in the dashboard */
export interface FaasTemplate {
  id: string
  name: string
  description: string
  tags: string[]
  /** Container port the workload exposes (documentation) */
  port: number
  /** Relative path to hit for health / demo */
  healthPath: string
  testMethod: 'GET' | 'POST'
  testPath: string
  /** Example JSON body for POST tests */
  testBodyExample: Record<string, unknown>
}

/** Running or historical container record */
export interface FaasContainer {
  id: string
  templateId: string
  templateName: string
  status: FaasContainerStatus
  createdAt: string
  updatedAt: string
  endpointUrl?: string
  railway?: {
    serviceId: string
    environmentId: string
    projectId?: string
    deploymentId?: string
  }
  errorMessage?: string
  logs: FaasLogLine[]
}
