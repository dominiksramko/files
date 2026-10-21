import { describe, expect, it } from 'vitest'
import { containScale, fitScale, placeholderSize } from '@/utils/imageFit'

const screen = { width: 1600, height: 900 }

describe('imageFit', () => {
  it('fits a picture into the container whole', () => {
    expect(containScale({ width: 3200, height: 900 }, screen)).toBe(0.5)
    expect(containScale({ width: 400, height: 450 }, screen)).toBe(2)
    expect(containScale({ width: 0, height: 0 }, screen)).toBe(1)
    expect(containScale({ width: 400, height: 300 }, { width: 0, height: 0 })).toBe(1)
  })

  it('never enlarges a picture in the viewer', () => {
    expect(fitScale({ width: 3200, height: 900 }, screen)).toBe(0.5)
    expect(fitScale({ width: 400, height: 450 }, screen)).toBe(1)
  })

  it('stretches a full-size placeholder over where the larger picture will be', () => {
    expect(placeholderSize({ width: 360, height: 640 }, screen)).toEqual({ width: 506.25, height: 900 })
    expect(placeholderSize({ width: 640, height: 360 }, screen)).toEqual({ width: 1600, height: 900 })
  })

  it('shows a smaller placeholder at the size the picture itself will have', () => {
    expect(placeholderSize({ width: 400, height: 300 }, screen)).toEqual({ width: 400, height: 300 })
    expect(placeholderSize({ width: 600, height: 300 }, { width: 300, height: 900 })).toEqual({ width: 300, height: 150 })
  })
})
