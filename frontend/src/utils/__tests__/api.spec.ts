// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ConflictError, fileops } from '@/utils/api'

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
