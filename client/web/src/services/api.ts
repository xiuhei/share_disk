export interface Account { id: string; account: string; created_at: string }
export interface BrowserSession { account: Account; csrf_token: string; expires_at: string }
export interface Replica { device_id: string; device_name: string; platform: string; state: string; online: boolean; is_origin: boolean }
export interface FileEntry {
  id: string; folder_id: string; name: string; size: number; mime: string; sha256: string;
  status: string; version: number; available: boolean; replicas: Replica[];
  origin_device_id: string; origin_device_name: string; created_at: string; updated_at: string;
  deleted_at?: string; purge_after?: string
}
export interface FolderEntry { id: string; parent_id?: string; name: string; updated_at: string; version: number }
export interface DeviceEntry { id: string; name: string; platform: string; status: string; last_seen_at?: string }
export interface TransferEntry { id: string; object_id: string; target_device_id: string; state: string; attempt: number; version: number; created_at: string }
export interface ShareEntry { id: string; file_id: string; file_name: string; token?: string; status: string; expires_at: string; created_at: string; download_count: number }
export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string) { super(message) }
}
export class ControlClient {
  private csrfToken = ''
  async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const headers = new Headers(options.headers)
    if (options.body) headers.set('Content-Type', 'application/json')
    if (options.method && options.method !== 'GET') headers.set('X-CSRF-Token', this.csrfToken)
    const response = await fetch(`/v1${path}`, { ...options, headers, credentials: 'same-origin', redirect: 'error' })
    if (!response.ok) {
      const body = await response.json().catch(() => null)
      if (response.status === 401 && path !== '/browser/login' && path !== '/browser/session') window.dispatchEvent(new Event('share-disk-session-expired'))
      throw new ApiError(response.status, body?.error?.code || 'HTTP_ERROR', body?.error?.message || `请求失败 (${response.status})`)
    }
    if (response.status === 204) return undefined as T
    return response.json() as Promise<T>
  }
  async session() {
    const session = await this.request<BrowserSession>('/browser/session')
    this.csrfToken = session.csrf_token
    return session
  }
  async login(account: string, password: string, remember = false) {
    const session = await this.request<BrowserSession>('/browser/login', { method: 'POST', body: JSON.stringify({ account, password, remember }) })
    this.csrfToken = session.csrf_token
    return session
  }
  async logout() { await this.request<void>('/browser/logout', { method: 'POST' }); this.csrfToken = '' }
  changePassword(current_password: string, new_password: string) { return this.request('/account/password', { method: 'PATCH', body: JSON.stringify({ current_password, new_password }) }) }
  async listFiles(folderId?: string, query = '', sort = 'newest') {
    const params = new URLSearchParams({ q: query, sort })
    if (folderId) params.set('folder_id', folderId)
    return (await this.request<{ files: FileEntry[] }>(`/catalog/files?${params}`)).files || []
  }
  getFile(id: string) { return this.request<FileEntry>(`/catalog/files/${encodeURIComponent(id)}`) }
  downloadTicket(id: string) { return this.request<{ url: string; ticket: string; expires_in: number }>(`/browser/files/${encodeURIComponent(id)}/download-ticket`, { method: 'POST' }) }
  async listFolders() { return (await this.request<{ folders: FolderEntry[] }>('/catalog/folders')).folders || [] }
  createFolder(name: string, parent_id?: string) { return this.request<FolderEntry>('/catalog/folders', { method: 'POST', body: JSON.stringify({ name, parent_id }) }) }
  renameFolder(id: string, name: string) { return this.request(`/catalog/folders/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify({ name }) }) }
  deleteFolder(id: string) { return this.request<void>(`/catalog/folders/${encodeURIComponent(id)}`, { method: 'DELETE' }) }
  fileAction(file_ids: string[], action: 'rename' | 'trash' | 'restore' | 'purge', name?: string, expected_versions?: Record<string, number>) {
    return this.request('/catalog/files/actions', { method: 'POST', body: JSON.stringify({ file_ids, action, name, expected_versions, operation_id: crypto.randomUUID() }) })
  }
  moveFiles(file_ids: string[], folder_id: string, expected_versions?: Record<string, number>) { return this.request<void>('/catalog/files/move', { method: 'POST', body: JSON.stringify({ file_ids, folder_id, expected_versions }) }) }
  async listTrash() { return (await this.request<{ files: FileEntry[] }>('/catalog/trash')).files || [] }
  async listDevices() { return (await this.request<{ devices: DeviceEntry[] }>('/devices')).devices || [] }
  removeDevice(id: string) { return this.request<void>(`/devices/${encodeURIComponent(id)}`, { method: 'DELETE' }) }
  renameDevice(id: string, name: string) { return this.request<void>(`/devices/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify({ name }) }) }
  async listTransfers() { return (await this.request<{ transfers: TransferEntry[] }>('/transfers')).transfers || [] }
  replicateFile(file_id: string, target_device_id: string) { return this.request<TransferEntry>('/transfers', { method: 'POST', body: JSON.stringify({ file_id, target_device_id }) }) }
  cancelTransfer(id: string) { return this.request<void>(`/transfers/${encodeURIComponent(id)}/cancel`, { method: 'POST' }) }
  async listShares() { return (await this.request<{ shares: ShareEntry[] }>('/shares')).shares || [] }
  createShare(file_id: string, expires_in_seconds: number) { return this.request<ShareEntry>('/shares', { method: 'POST', body: JSON.stringify({ file_id, expires_in_seconds }) }) }
  deleteShare(id: string) { return this.request<void>(`/shares/${encodeURIComponent(id)}`, { method: 'DELETE' }) }
}
export const api = new ControlClient()
export default api
