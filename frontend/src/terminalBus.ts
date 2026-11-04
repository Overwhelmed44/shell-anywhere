type Listener = (data: Uint8Array) => void
const listeners = new Map<string, Listener>()
const pending = new Map<string, string[]>()

export function pushTerminalData(id: string, data: string) {
  const bytes = Uint8Array.from(atob(data), character => character.charCodeAt(0))
  const listener = listeners.get(id)
  if (listener) { listener(bytes); return }
  const buffer = pending.get(id) ?? []
  buffer.push(data)
  if (buffer.length > 200) buffer.shift()
  pending.set(id, buffer)
}

export function subscribeTerminal(id: string, listener: Listener) {
  listeners.set(id, listener)
  for (const chunk of pending.get(id) ?? []) listener(Uint8Array.from(atob(chunk), character => character.charCodeAt(0)))
  pending.delete(id)
  return () => { listeners.delete(id) }
}

export function clearTerminal(id: string) { listeners.delete(id); pending.delete(id) }
