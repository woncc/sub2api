import { apiClient } from './client'

export type UserOSSProvider = 'aliyun' | 'tencent' | 'qiniuyun' | 'cloudflare' | 's3'

export interface UserOSSRepository {
  id: number
  provider: UserOSSProvider
  bucket: string
  domain: string
  region?: string
  endpoint?: string
  access_key_id?: string
  account_id?: string
  bucket_url?: string
  force_path_style?: boolean
  secret_configured: boolean
  created_at: string
  updated_at: string
}

export interface UserOSSInput {
  provider: UserOSSProvider
  access_key_id?: string
  access_key_secret?: string
  bucket?: string
  region?: string
  domain?: string
  secret_id?: string
  secret_key?: string
  bucket_url?: string
  access_key?: string
  account_id?: string
  bucket_name?: string
  endpoint?: string
  secret_access_key?: string
  force_path_style?: boolean
}

export interface UserOSSCheckResult {
  ok: boolean
  message: string
}

export async function listOSSRepositories(): Promise<UserOSSRepository[]> {
  const { data } = await apiClient.get<{ items: UserOSSRepository[] }>('/user/oss')
  return data.items ?? []
}

export async function createOSSRepository(input: UserOSSInput): Promise<UserOSSRepository> {
  const { data } = await apiClient.post<UserOSSRepository>('/user/oss', input)
  return data
}

export async function updateOSSRepository(id: number, input: UserOSSInput): Promise<UserOSSRepository> {
  const { data } = await apiClient.put<UserOSSRepository>(`/user/oss/${id}`, input)
  return data
}

export async function deleteOSSRepository(id: number): Promise<void> {
  await apiClient.delete(`/user/oss/${id}`)
}

export async function checkOSSRepository(id: number, input?: UserOSSInput): Promise<UserOSSCheckResult> {
  const { data } = await apiClient.post<UserOSSCheckResult>(`/user/oss/${id}/check`, input ?? {})
  return data
}
