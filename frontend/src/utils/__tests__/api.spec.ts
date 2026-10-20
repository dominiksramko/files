// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ConflictError, fileops, files, onSessionEnded } from '@/utils/api'

afterEach(() => vi.unstubAllGlobals())

describe('fileops.rename', () => {
  it('sends the conflict mode and returns the final name', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ message: 'renamed', name: 'final (1).txt' })))
    vi.stubGlobal('fetch', fetchMock)
    expect(await fileops.rename('docs/notes.txt', 'final.txt', 'rename')).toEqual({ message: 'renamed', name: 'final (1).txt' })
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('/api/v1/files/rename')
    expect(JSON.parse(init.body as string)).toEqual({ path: 'docs/notes.txt', name: 'final.txt', conflict_mode: 'rename' })
  })

  it('throws ConflictError when the name is taken', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ conflicts: ['final.txt'] }), { status: 409 })))
    const error = await fileops.rename('notes.txt', 'final.txt').catch((err) => err)
    expect(error).toBeInstanceOf(ConflictError)
    expect(error.conflicts).toEqual(['final.txt'])
  })
})

// A stand-in XMLHttpRequest answering every request with status and body.
function fakeXhr(status: number, body: unknown) {
  const opened: string[] = []
  class FakeXhr {
    status = status
    responseText = JSON.stringify(body)
    upload = {}
    onload: (() => void) | null = null
    open(_method: string, url: string) { opened.push(url) }
    setRequestHeader() {}
    send() { setTimeout(() => this.onload?.()) }
    abort() {}
  }
  vi.stubGlobal('XMLHttpRequest', FakeXhr)
  return opened
}

describe('fileops.upload', () => {
  it('sends the folder and conflict mode', async () => {
    const opened = fakeXhr(200, { message: 'uploaded', count: 1 })
    const { promise } = fileops.upload('my docs', [new File(['x'], 'a.txt')], undefined, 'replace')
    expect(await promise).toEqual({ message: 'uploaded', count: 1 })
    const url = new URL(opened[0]!, 'http://localhost')
    expect(url.pathname).toBe('/api/v1/files/upload')
    expect(url.searchParams.get('path')).toBe('my docs')
    expect(url.searchParams.get('conflict_mode')).toBe('replace')
  })

  it('throws ConflictError when names are taken', async () => {
    fakeXhr(409, { error: 'conflict', conflicts: ['a.txt'] })
    const error = await fileops.upload('', [new File(['x'], 'a.txt')]).promise.catch((err) => err)
    expect(error).toBeInstanceOf(ConflictError)
    expect(error.conflicts).toEqual(['a.txt'])
  })
})

describe('no login token in requests', () => {
  it('URLs for media, downloads and streams carry none', () => {
    const urls = [
      files.downloadUrl('a/b.png', '2026-01-01T00:00:00Z'),
      files.streamUrl('a/b.png', '2026-01-01T00:00:00Z'),
      files.streamUrlByPath('a/b.png'),
      files.hlsPlaylistUrl('a/b.avi'),
      files.thumbUrl('a/b.png', 'hash'),
      files.searchStreamUrl('query', 'a'),
      files.sizeStreamUrl(['a']),
    ]
    for (const url of urls) expect(url).not.toMatch(/token/i)
  })

  it('API calls send no Authorization header (the session cookie goes along by itself)', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ message: 'ok' })))
    vi.stubGlobal('fetch', fetchMock)
    await fileops.delete(['x'])
    const [, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(new Headers(init.headers).has('Authorization')).toBe(false)
  })
})

describe('onSessionEnded', () => {
  afterEach(() => onSessionEnded(() => {}))

  it('runs when the server answers 401, and only then', async () => {
    const ended = vi.fn()
    onSessionEnded(ended)
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: 'nope' }), { status: 403 })))
    await fileops.delete(['x']).catch(() => {})
    expect(ended).not.toHaveBeenCalled()

    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: 'not logged in' }), { status: 401 })))
    await expect(fileops.delete(['x'])).rejects.toThrow('not logged in')
    await files.readContent('notes.txt').catch(() => {})
    await files.writeContent('notes.txt', 'x', 'v1').catch(() => {})
    expect(ended).toHaveBeenCalledTimes(3)
  })

  it('runs for uploads too', async () => {
    const ended = vi.fn()
    onSessionEnded(ended)
    fakeXhr(401, { error: 'not logged in' })
    await fileops.upload('', [new File(['x'], 'a.txt')]).promise.catch(() => {})
    expect(ended).toHaveBeenCalledOnce()
  })
})
