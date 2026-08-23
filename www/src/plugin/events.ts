export type PluginEventPayload = unknown[];
export type PluginEventHandler = (...args: PluginEventPayload) => void;

const listeners = new Map<string, Set<PluginEventHandler>>();

export function emit(event: string, ...args: PluginEventPayload): void {
  const set = listeners.get(event);
  if (!set) return;
  set.forEach((cb) => cb(...args));
}

export function on(event: string, callback: PluginEventHandler): () => void {
  if (!listeners.has(event)) listeners.set(event, new Set());
  listeners.get(event)!.add(callback);
  return () => off(event, callback);
}

export function off(event: string, callback: PluginEventHandler): void {
  listeners.get(event)?.delete(callback);
}
