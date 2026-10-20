import { describe, expect, it } from 'vitest'
import { droppedFiles } from '../drop'

function fileItem(name: string, isDirectory = false): DataTransferItem {
  const file = new File(['content'], name)
  return {
    kind: 'file',
    type: '',
    getAsFile: () => file,
    webkitGetAsEntry: () => ({ name, isDirectory, isFile: !isDirectory }),
  } as unknown as DataTransferItem
}

function transfer(...items: DataTransferItem[]): DataTransfer {
  return { items } as unknown as DataTransfer
}

describe('droppedFiles', () => {
  it('keeps files and leaves out folders', () => {
    const dropped = droppedFiles(transfer(fileItem('a.txt'), fileItem('photos', true), fileItem('b.txt')))
    expect(dropped.files.map((file) => file.name)).toEqual(['a.txt', 'b.txt'])
    expect(dropped.folders).toEqual(['photos'])
  })

  it('ignores dragged text', () => {
    const text = { kind: 'string', type: 'text/plain' } as DataTransferItem
    expect(droppedFiles(transfer(text, fileItem('a.txt'))).files).toHaveLength(1)
  })

  it('takes every file where entries are not available', () => {
    const item = { kind: 'file', type: '', getAsFile: () => new File([''], 'a.txt') } as unknown as DataTransferItem
    expect(droppedFiles(transfer(item))).toEqual({ files: [expect.any(File)], folders: [] })
  })
})
