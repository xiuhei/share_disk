import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, ControlClient } from './api'

afterEach(() => vi.unstubAllGlobals())
describe('ControlClient wire contract', () => {
  it('uses opaque cookie sessions and sends the returned CSRF token on writes', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({ account: { id: 'u1', account: 'alice' }, csrf_token: 'csrf', expires_at: '2026-09-12' })))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)
    const api = new ControlClient()
    await api.login('alice', 'secret', true)
    expect(fetchMock.mock.calls[0][0]).toBe('/v1/browser/login')
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ account: 'alice', password: 'secret', remember: true })
    await expect(api.removeDevice('device/id')).resolves.toBeUndefined()
    const [url, request] = fetchMock.mock.calls[1]
    expect(url).toBe('/v1/devices/device%2Fid')
    expect(request.credentials).toBe('same-origin')
    expect(request.redirect).toBe('error')
    expect(request.headers.get('X-CSRF-Token')).toBe('csrf')
    expect(request.headers.has('Authorization')).toBe(false)
  })
  it('handles nested errors without hiding server conflicts', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { code: 'NAME_CONFLICT', message: 'Name already exists' } }), { status: 409 })))
    await expect(new ControlClient().createFolder('duplicate')).rejects.toMatchObject({ status: 409, code: 'NAME_CONFLICT', message: 'Name already exists' })
  })
  it('does not treat HTML error pages as a successful operation', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('<h1>Bad gateway</h1>', { status: 502 })))
    await expect(new ControlClient().listFolders()).rejects.toBeInstanceOf(ApiError)
  })
  it('maps list wrappers and preserves search characters', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ files: [{ id: 'f1' }] })))
    vi.stubGlobal('fetch', fetchMock)
    expect(await new ControlClient().listFiles('folder/a', '照片 & notes', 'oldest')).toEqual([{ id: 'f1' }])
    const url = new URL(fetchMock.mock.calls[0][0], 'http://localhost')
    expect(url.pathname).toBe('/v1/catalog/files')
    expect(url.searchParams.get('folder_id')).toBe('folder/a')
    expect(url.searchParams.get('q')).toBe('照片 & notes')
    expect(url.searchParams.get('sort')).toBe('oldest')
  })
  it('uses the documented actions and a unique idempotency key', async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(new Response('{}')))
    vi.stubGlobal('fetch', fetchMock)
    const api = new ControlClient()
    await api.fileAction(['f1'], 'restore')
    await api.fileAction(['f1'], 'trash')
    const first = JSON.parse(fetchMock.mock.calls[0][1].body)
    const second = JSON.parse(fetchMock.mock.calls[1][1].body)
    expect(first).toMatchObject({ file_ids: ['f1'], action: 'restore' })
    expect(first.operation_id).not.toBe(second.operation_id)
  })
})
