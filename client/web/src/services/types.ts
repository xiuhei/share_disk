export interface File {
  id: string
  name: string
  size: number
  mime: string
  sha256: string
  status: string
  createdAt: string
  modifiedAt: string
  isFolder: boolean
  parentId?: string
  originDeviceId?: string
  originDeviceName?: string
  available?: boolean
  replicas?: FileReplica[]
}

export interface FileReplica {
  deviceId: string
  deviceName: string
  platform: string
  state: 'pending' | 'ready' | 'missing' | 'corrupt' | 'deleting' | 'deleted'
  online: boolean
  isOrigin: boolean
  lastSeenAt?: string
}

export interface Device {
  id: string
  name: string
  type: 'desktop' | 'mobile' | 'server'
  online: boolean
  lastSeen: string
  ip?: string
  storage: {
    used: number
    total: number
  }
}

export interface Transfer {
  id: string
  fileId: string
  fileName: string
  type: 'upload' | 'download'
  status: 'pending' | 'active' | 'paused' | 'completed' | 'error'
  progress: number
  speed: number
  size: number
  createdAt: string
}

export interface Share {
  id: string
  fileId: string
  fileName: string
  url: string
  createdAt: string
  expiresAt: string
  downloads: number
  password?: string
}

export interface User {
  id: string
  username: string
  email?: string
  createdAt: string
}

export interface ApiResponse<T> {
  data: T
  error?: string
}
