import { File, Device, Transfer, Share, User, ApiResponse } from './types'

const API_BASE = '/api'

class ApiClient {
  private token: string | null = null

  setToken(token: string | null) {
    this.token = token
  }

  private async request<T>(path: string, options?: RequestInit): Promise<T> {
    const headers: HeadersInit = {
      'Content-Type': 'application/json',
    }
    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`
    }

    const response = await fetch(`${API_BASE}${path}`, {
      ...options,
      headers: {
        ...headers,
        ...options?.headers,
      },
    })

    if (!response.ok) {
      const error = await response.json().catch(() => ({ message: '请求失败' }))
      throw new Error(error.message || '请求失败')
    }

    return response.json()
  }

  // Auth
  async login(username: string, password: string): Promise<{ token: string; user: User }> {
    return this.request('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    })
  }

  async bootstrap(token: string, username: string, password: string): Promise<{ token: string; user: User }> {
    return this.request('/auth/bootstrap', {
      method: 'POST',
      body: JSON.stringify({ token, username, password }),
    })
  }

  // Files
  async listFiles(parentId?: string): Promise<File[]> {
    const params = parentId ? `?parentId=${parentId}` : ''
    return this.request(`/catalog/files${params}`)
  }

  async getFile(id: string): Promise<File> {
    return this.request(`/catalog/files/${id}`)
  }

  async createFolder(name: string, parentId?: string): Promise<File> {
    return this.request('/catalog/folders', {
      method: 'POST',
      body: JSON.stringify({ name, parentId }),
    })
  }

  async renameFile(id: string, name: string): Promise<void> {
    await this.request(`/catalog/files/${id}/rename`, {
      method: 'PUT',
      body: JSON.stringify({ name }),
    })
  }

  async moveFile(id: string, parentId: string): Promise<void> {
    await this.request(`/catalog/files/${id}/move`, {
      method: 'PUT',
      body: JSON.stringify({ parentId }),
    })
  }

  async deleteFile(id: string): Promise<void> {
    await this.request(`/catalog/files/${id}`, {
      method: 'DELETE',
    })
  }

  async trashFile(id: string): Promise<void> {
    await this.request(`/catalog/files/${id}/trash`, {
      method: 'PUT',
    })
  }

  async restoreFile(id: string): Promise<void> {
    await this.request(`/catalog/trash/${id}/restore`, {
      method: 'PUT',
    })
  }

  // Transfers
  async listTransfers(): Promise<Transfer[]> {
    return this.request('/transfers')
  }

  async cancelTransfer(id: string): Promise<void> {
    await this.request(`/transfers/${id}`, {
      method: 'DELETE',
    })
  }

  // Devices
  async listDevices(): Promise<Device[]> {
    return this.request('/devices')
  }

  async removeDevice(id: string): Promise<void> {
    await this.request(`/devices/${id}`, {
      method: 'DELETE',
    })
  }

  // Shares
  async listShares(): Promise<Share[]> {
    return this.request('/shares')
  }

  async createShare(fileId: string, expiresIn?: number, password?: string): Promise<Share> {
    return this.request('/shares', {
      method: 'POST',
      body: JSON.stringify({ fileId, expiresIn, password }),
    })
  }

  async deleteShare(id: string): Promise<void> {
    await this.request(`/shares/${id}`, {
      method: 'DELETE',
    })
  }

  // Upload (tus)
  async uploadFile(file: File, onProgress?: (progress: number) => void): Promise<string> {
    // TODO: Implement tus upload
    return new Promise((resolve) => {
      let progress = 0
      const interval = setInterval(() => {
        progress += 10
        onProgress?.(progress)
        if (progress >= 100) {
          clearInterval(interval)
          resolve('file-id')
        }
      }, 100)
    })
  }

  // Download
  async downloadFile(id: string, name: string): Promise<Blob> {
    const response = await fetch(`${API_BASE}/catalog/files/${id}/download`, {
      headers: this.token ? { Authorization: `Bearer ${this.token}` } : {},
    })
    if (!response.ok) throw new Error('下载失败')
    return response.blob()
  }
}

export const api = new ApiClient()
export default api
