import { describe, expect, it } from 'vitest'
import { slideStart, translateXOf } from '@/utils/carousel'

describe('slideStart', () => {
  it('starts a slide from the middle one panel over, where the picture was', () => {
    expect(slideStart(0, 'next', 800)).toBe(800)
    expect(slideStart(0, 'prev', 800)).toBe(-800)
  })

  it('carries on from where a running slide or a drag has the track', () => {
    expect(slideStart(300, 'next', 800)).toBe(800) // one panel at most
    expect(slideStart(-300, 'next', 800)).toBe(500) // dragged part of the way
    expect(slideStart(300, 'prev', 800)).toBe(-500)
  })

  it('never starts more than one panel away', () => {
    expect(slideStart(799, 'next', 800)).toBe(800)
    expect(slideStart(-799, 'prev', 800)).toBe(-800)
  })
})

describe('translateXOf', () => {
  it('reads the translation of 2D and 3D matrices', () => {
    expect(translateXOf('matrix(1, 0, 0, 1, -1200.5, 0)')).toBe(-1200.5)
    expect(translateXOf('matrix3d(1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, -640, 0, 0, 1)')).toBe(-640)
  })

  it('reads no transform as no translation', () => {
    expect(translateXOf('none')).toBe(0)
  })

  it("refuses what it can't read", () => {
    expect(translateXOf('translateX(calc(-33.333% + 0px))')).toBeNull()
    expect(translateXOf('matrix(1, 0, 0, 1)')).toBeNull()
    expect(translateXOf('matrix(1, 0, 0, 1, abc, 0)')).toBeNull()
    expect(translateXOf('')).toBeNull()
  })
})
