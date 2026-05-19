import { railwayGraphQL } from './graphql'

/** Railway deployment node (subset used by FaaS dashboard) */
export interface DeploymentNode {
  id: string
  status: string
  url?: string | null
  staticUrl?: string | null
  createdAt?: string | null
}

interface DeploymentsQueryResult {
  deployments: {
    edges: Array<{ node: DeploymentNode }>
  }
}

const DEPLOYMENTS_QUERY = `
query FaasDeployments($input: DeploymentListInput!, $first: Int!) {
  deployments(input: $input, first: $first) {
    edges {
      node {
        id
        status
        url
        staticUrl
        createdAt
      }
    }
  }
}`

export async function listRecentDeployments(input: Record<string, unknown>, first = 8) {
  const data = await railwayGraphQL<DeploymentsQueryResult>(DEPLOYMENTS_QUERY, {
    input,
    first,
  })
  return data.deployments.edges.map((e) => e.node)
}

function sortDeployments(nodes: DeploymentNode[]) {
  return [...nodes].sort((a, b) => {
    const ta = a.createdAt ? Date.parse(a.createdAt) : 0
    const tb = b.createdAt ? Date.parse(b.createdAt) : 0
    return tb - ta
  })
}

interface DeployMutationObject {
  serviceInstanceDeploy: DeploymentNode
}

const DEPLOY_WITH_SELECTION = `
mutation FaasServiceDeploySel($environmentId: String!, $serviceId: String!) {
  serviceInstanceDeploy(
    environmentId: $environmentId
    serviceId: $serviceId
    latestCommit: true
  ) {
    id
    status
    url
    staticUrl
    createdAt
  }
}`

const DEPLOY_SCALAR = `
mutation FaasServiceDeployScalar($environmentId: String!, $serviceId: String!) {
  serviceInstanceDeploy(
    environmentId: $environmentId
    serviceId: $serviceId
    latestCommit: true
  )
}`

export async function triggerServiceDeploy(serviceId: string, environmentId: string): Promise<DeploymentNode | null> {
  try {
    const data = await railwayGraphQL<DeployMutationObject>(DEPLOY_WITH_SELECTION, {
      serviceId,
      environmentId,
    })
    return data.serviceInstanceDeploy ?? null
  } catch (first) {
    try {
      await railwayGraphQL<{ serviceInstanceDeploy: unknown }>(DEPLOY_SCALAR, {
        serviceId,
        environmentId,
      })
      const input: Record<string, unknown> = { serviceId, environmentId }
      const nodes = sortDeployments(await listRecentDeployments(input, 8))
      return nodes[0] ?? null
    } catch (second) {
      const a = first instanceof Error ? first.message : String(first)
      const b = second instanceof Error ? second.message : String(second)
      throw new Error(`${a} | fallback: ${b}`)
    }
  }
}

interface DeploymentStopResult {
  deploymentStop: boolean
}

const STOP_DEPLOYMENT = `
mutation FaasDeploymentStop($id: String!) {
  deploymentStop(id: $id)
}`

export async function stopDeployment(deploymentId: string): Promise<boolean> {
  const data = await railwayGraphQL<DeploymentStopResult>(STOP_DEPLOYMENT, { id: deploymentId })
  return Boolean(data.deploymentStop)
}

interface DeploymentLogsResult {
  deploymentLogs: Array<{
    message: string
    severity?: string | null
    timestamp?: string | null
  }>
}

const LOGS_QUERY = `
query FaasDeploymentLogs($deploymentId: String!, $limit: Int!) {
  deploymentLogs(deploymentId: $deploymentId, limit: $limit) {
    message
    severity
    timestamp
  }
}`

export async function fetchDeploymentLogs(deploymentId: string, limit = 80) {
  const data = await railwayGraphQL<DeploymentLogsResult>(LOGS_QUERY, {
    deploymentId,
    limit,
  })
  return data.deploymentLogs ?? []
}
