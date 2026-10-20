// Files dropped from the desktop. A dropped folder shows up in
// DataTransfer.files as a file that can't be read, and uploading it fails the
// whole upload — so folders are told apart by their entries and left out.
export interface DroppedFiles {
  files: File[]
  folders: string[]
}

export function droppedFiles(dataTransfer: DataTransfer): DroppedFiles {
  const dropped: DroppedFiles = { files: [], folders: [] }
  for (const item of Array.from(dataTransfer.items)) {
    if (item.kind !== 'file') continue
    const file = item.getAsFile()
    const entry = item.webkitGetAsEntry?.()
    if (entry?.isDirectory) dropped.folders.push(entry.name)
    else if (file) dropped.files.push(file)
  }
  return dropped
}
