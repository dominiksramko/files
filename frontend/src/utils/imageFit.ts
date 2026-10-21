export interface Size {
  width: number
  height: number
}

// The longest side of a preview-sized thumbnail (PreviewMaxDim in the
// backend's thumbs package).
export const PREVIEW_THUMB_MAX_SIDE = 640

/** The scale that fits a picture into the container whole, enlarging it if need be (1 while either size is unknown). */
export function containScale(picture: Size, container: Size): number {
  if (!picture.width || !picture.height || !container.width || !container.height) return 1
  return Math.min(container.width / picture.width, container.height / picture.height)
}

/** The scale the image viewer shows a picture at: fitted into the container, never enlarged. */
export function fitScale(picture: Size, container: Size): number {
  return Math.min(1, containScale(picture, container))
}

/**
 * The size to show a preview-sized thumbnail at while its file loads, so it
 * covers where the file will appear. A thumbnail at the full size stands for
 * a larger picture, which the viewer fits into the container; a smaller one
 * is the picture's own size, shown as the viewer will show the file.
 */
export function placeholderSize(thumb: Size, container: Size): Size {
  const standsForLarger = Math.max(thumb.width, thumb.height) >= PREVIEW_THUMB_MAX_SIDE
  const scale = standsForLarger ? containScale(thumb, container) : fitScale(thumb, container)
  return { width: thumb.width * scale, height: thumb.height * scale }
}
