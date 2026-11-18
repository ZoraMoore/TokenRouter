/**
 * Admin TLS Fingerprint Profile API endpoints
 * Handles TLS fingerprint profile CRUD for administrators
 */

import { apiClient } from '../client'

/**
 * TLS fingerprint profile interface
 */
export interface TLSFingerprintProfile {
  id: number
  name: string
  description: string | null
  enable_grease: boolean
  cipher_suites: number[]
  curves: number[]
  point_formats: number[]
  signature_algorithms: number[]
  signature_algorithms_cert: number[]
  alpn_protocols: string[]
  supported_versions: number[]
  key_share_groups: number[]
  psk_modes: number[]
  extensions: number[]
  verification: TLSFingerprintProfileVerification
  created_at: string
  updated_at: string
}

export interface TLSFingerprintTargetValidation {
  host: string
  success: boolean
  negotiated_version?: number
  cipher_suite?: number
  negotiated_alpn?: string
  error?: string
  validated_at: string
}

export interface TLSFingerprintProfileVerification {
  source_kind: 'manual' | 'controlled_connect' | 'authenticated_ingress' | 'legacy'
  client_kind?: string
  client_version?: string
  runtime?: string
  runtime_version?: string
  operating_system?: string
  architecture?: string
  capture_target?: string
  observed_host?: string
  independent_connections?: number
  classifier_evidence?: string[]
  approved_for_use?: boolean
  random_pool_eligible?: boolean
  validation_status: 'draft' | 'verified' | 'rejected' | 'stale'
  validator_version: number
  observed_ja3_hash?: string
  replayed_ja3_hash?: string
  fingerprint_sha256?: string
  issues?: string[]
  warnings?: string[]
  validated_targets?: TLSFingerprintTargetValidation[]
  validated_at?: string
}

/**
 * Create profile request
 */
export interface CreateProfileRequest {
  name: string
  description?: string | null
  enable_grease?: boolean
  cipher_suites?: number[]
  curves?: number[]
  point_formats?: number[]
  signature_algorithms?: number[]
  signature_algorithms_cert?: number[]
  alpn_protocols?: string[]
  supported_versions?: number[]
  key_share_groups?: number[]
  psk_modes?: number[]
  extensions?: number[]
}

/**
 * Update profile request
 */
export interface UpdateProfileRequest {
  name?: string
  description?: string | null
  enable_grease?: boolean
  cipher_suites?: number[]
  curves?: number[]
  point_formats?: number[]
  signature_algorithms?: number[]
  signature_algorithms_cert?: number[]
  alpn_protocols?: string[]
  supported_versions?: number[]
  key_share_groups?: number[]
  psk_modes?: number[]
  extensions?: number[]
}

export interface TLSFingerprintCollectorStatus {
  running: boolean
  listen_address: string
  public_base_url: string
  using_generated_cert: boolean
  ca_pem?: string
  session_ttl_seconds: number
  max_records_per_session: number
  started_at?: string
  last_error?: string
}

/**
 * 已认证入站 TLS 观测链路的运行快照。
 * 这里只返回聚合计数与运行状态，不包含客户标识或原始握手内容。
 */
export interface TLSFingerprintAuthenticatedIngressStatus {
  enabled: boolean
  pending_client_hellos: number
  pending_authenticated_flows: number
  candidate_groups: number
  validations_in_flight: number
  matched_flows: number
  rejected_flows: number
  verified_profiles_created: number
  existing_profiles_matched: number
  last_verified_at?: string
  last_error?: string
}

export interface TLSFingerprintCollectorSession {
  token: string
  expires_at: string
  capture_url: string
  proxy_url?: string
  mode: string
  client_kind?: string
  client_version?: string
  runtime?: string
  runtime_version?: string
  operating_system?: string
  architecture?: string
  target_host?: string
  cold_start_confirmed?: boolean
  ca_pem?: string
}

export interface CreateTLSFingerprintCollectorSessionRequest {
  mode: 'controlled_connect'
  client_kind: 'codex' | 'node' | 'claude_code'
  client_version: string
  runtime?: string
  runtime_version?: string
  operating_system: string
  architecture: string
  target_host: 'chatgpt.com' | 'api.openai.com'
  cold_start_confirmed: boolean
}

export interface TLSFingerprintReplayValidation {
  reusable: boolean
  exact_ja3: boolean
  exact_structure: boolean
  captured_ja3_hash: string
  replayed_ja3_hash: string
  issues: Array<{ code: string; message: string }>
  warnings: Array<{ code: string; message: string }>
}

export interface TLSFingerprintCaptureRecord {
  id: string
  captured_at: string
  client_kind: string
  detected_client_kind?: string
  source_kind?: string
  capture_mode?: string
  target_host?: string
  promotion_eligible?: boolean
  matching_sample_count?: number
  blocked_reasons?: string[]
  request_path: string
  method: string
  user_agent: string
  ja3_raw: string
  ja3_hash: string
  negotiated_alpn: string
  http_proto: string
  profile: TLSFingerprintProfile
  yaml: string
  headers_summary: Record<string, string>
  stainless_summary: Record<string, string>
  replay_validation?: TLSFingerprintReplayValidation
}

export async function list(options: { bindable?: boolean; targetHost?: string } = {}): Promise<TLSFingerprintProfile[]> {
  const { data } = await apiClient.get<TLSFingerprintProfile[]>('/admin/tls-fingerprint-profiles', {
    params: options.bindable ? { bindable: true, target_host: options.targetHost } : undefined
  })
  return data
}

export async function getById(id: number): Promise<TLSFingerprintProfile> {
  const { data } = await apiClient.get<TLSFingerprintProfile>(`/admin/tls-fingerprint-profiles/${id}`)
  return data
}

export async function create(profileData: CreateProfileRequest): Promise<TLSFingerprintProfile> {
  const { data } = await apiClient.post<TLSFingerprintProfile>('/admin/tls-fingerprint-profiles', profileData)
  return data
}

export async function update(id: number, updates: UpdateProfileRequest): Promise<TLSFingerprintProfile> {
  const { data } = await apiClient.put<TLSFingerprintProfile>(`/admin/tls-fingerprint-profiles/${id}`, updates)
  return data
}

export async function deleteProfile(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/tls-fingerprint-profiles/${id}`)
  return data
}

export async function setApproval(
  id: number,
  input: { approved_for_use: boolean; random_pool_eligible: boolean }
): Promise<TLSFingerprintProfile> {
  const { data } = await apiClient.post<TLSFingerprintProfile>(
    `/admin/tls-fingerprint-profiles/${id}/approval`,
    input
  )
  return data
}

export async function collectorStatus(): Promise<TLSFingerprintCollectorStatus> {
  const { data } = await apiClient.get<TLSFingerprintCollectorStatus>('/admin/tls-fingerprint-profiles/collector/status')
  return data
}

export async function authenticatedIngressStatus(): Promise<TLSFingerprintAuthenticatedIngressStatus> {
  const { data } = await apiClient.get<TLSFingerprintAuthenticatedIngressStatus>(
    '/admin/tls-fingerprint-profiles/collector/authenticated-ingress/status'
  )
  return data
}

export async function startCollector(): Promise<TLSFingerprintCollectorStatus> {
  const { data } = await apiClient.post<TLSFingerprintCollectorStatus>('/admin/tls-fingerprint-profiles/collector/start')
  return data
}

export async function stopCollector(): Promise<TLSFingerprintCollectorStatus> {
  const { data } = await apiClient.post<TLSFingerprintCollectorStatus>('/admin/tls-fingerprint-profiles/collector/stop')
  return data
}

export async function createCollectorSession(request: CreateTLSFingerprintCollectorSessionRequest): Promise<TLSFingerprintCollectorSession> {
  const { data } = await apiClient.post<TLSFingerprintCollectorSession>('/admin/tls-fingerprint-profiles/collector/sessions', request)
  return data
}

export async function promoteCollectorCapture(
  token: string,
  captureId: string,
  input: { name?: string; description?: string | null } = {}
): Promise<TLSFingerprintProfile> {
  const { data } = await apiClient.post<TLSFingerprintProfile>(
    `/admin/tls-fingerprint-profiles/collector/sessions/${encodeURIComponent(token)}/captures/${encodeURIComponent(captureId)}/promote`,
    input
  )
  return data
}

export async function listCollectorCaptures(token: string): Promise<TLSFingerprintCaptureRecord[]> {
  const { data } = await apiClient.get<TLSFingerprintCaptureRecord[]>(`/admin/tls-fingerprint-profiles/collector/sessions/${encodeURIComponent(token)}/captures`)
  return data
}

export async function deleteCollectorSession(token: string): Promise<{ deleted: boolean }> {
  const { data } = await apiClient.delete<{ deleted: boolean }>(`/admin/tls-fingerprint-profiles/collector/sessions/${encodeURIComponent(token)}`)
  return data
}

export const tlsFingerprintProfileAPI = {
  list,
  getById,
  create,
  update,
  delete: deleteProfile,
  setApproval,
  collectorStatus,
  authenticatedIngressStatus,
  startCollector,
  stopCollector,
  createCollectorSession,
  listCollectorCaptures,
  promoteCollectorCapture,
  deleteCollectorSession
}

export default tlsFingerprintProfileAPI
