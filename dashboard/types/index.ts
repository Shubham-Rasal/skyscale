export interface ExecutionContext {
  ExecutionID: string
  RequestID: string
  FunctionID: string
  FunctionName: string
  VMID: string
  StartTime: string
  Sync: boolean
  JobType: string
  HardwareType: string
  GPUModel: string
}

export interface Execution {
  ID: string
  FunctionID: string
  FunctionName: string
  Status: string
  StartTime: string
  EndTime: string
  CreatedAt: string
  UpdatedAt: string
  Duration: number
  VMID: string
  Logs: string
  Error: string
  JobType: string
  HardwareType: string
  GPUModel: string
}

export interface VM {
  ID: string
  Status: string
  IP: string
  CreatedAt: string
  LastUsed: string
  Memory: number
  CPU: number
  IsWarm: boolean
  HardwareType: string
  GPUModel: string
  VRAMmb: number
  AkashDeploymentID: string
  ProviderAddr: string
}

export interface GPUMetric {
  job_id: string
  gpu_model: string
  provider: string
  utilization_pct: number
  memory_total_mb: number
  memory_used_mb: number
  steps_per_sec: number
  updated_at: number
}

export interface TrainingMetric {
  job_id: string
  step: number
  episode_reward: number
  loss: number
  gpu_util: number
  timestamp: number
}

export interface WSPayload {
  active: ExecutionContext[]
  recent: Execution[]
  vm_pool: VM[]
  gpu: GPUMetric[]
  training_metrics: Record<string, TrainingMetric[]>
}

export interface Function {
  id: string
  name: string
  runtime: string
  memory: number
  timeout: number
  status: string
  version: string
}
