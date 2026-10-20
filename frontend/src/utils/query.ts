import type { LocationQuery } from 'vue-router'

// A key's text in the URL: undefined when it's not there, '' for a bare ?key.
export function queryString(value: LocationQuery[string] | undefined): string | undefined {
  if (value === undefined) return undefined
  const first = Array.isArray(value) ? value[0] : value
  return first ?? ''
}

// query with values set for keys: one left undefined is dropped, and the
// other keys stay. For the modals that keep their state next to the file
// browser's own keys.
export function withKeys(query: LocationQuery, keys: string[], values: Record<string, string | undefined>): LocationQuery {
  const rest = Object.fromEntries(Object.entries(query).filter(([key]) => !keys.includes(key)))
  const set = Object.entries(values).filter((entry): entry is [string, string] => entry[1] !== undefined)
  return { ...rest, ...Object.fromEntries(set) }
}
