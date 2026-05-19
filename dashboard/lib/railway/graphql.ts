import { getRailwayGraphqlUrl, getRailwayToken } from './config'

export interface RailwayGraphQLResponse<T> {
  data?: T
  errors?: Array<{ message: string }>
}

export async function railwayGraphQL<T>(
  query: string,
  variables?: Record<string, unknown>,
): Promise<T> {
  const token = getRailwayToken()
  if (!token) {
    throw new Error('RAILWAY_API_TOKEN is not set')
  }

  const res = await fetch(getRailwayGraphqlUrl(), {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({ query, variables }),
    cache: 'no-store',
  })

  const json = (await res.json()) as RailwayGraphQLResponse<T>
  if (!res.ok) {
    throw new Error(`Railway HTTP ${res.status}`)
  }
  if (json.errors?.length) {
    throw new Error(json.errors.map((e) => e.message).join('; '))
  }
  if (json.data === undefined) {
    throw new Error('Railway returned empty data')
  }
  return json.data
}
