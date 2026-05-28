import { reactive } from "vue";
import type { Component } from "vue";

export interface SlotEntry {
  component: Component;
  props?: (() => Record<string, any>) | Record<string, any>;
  events?: Record<string, (...args: any[]) => void>;
  order?: number;
}

export const slotRegistry = reactive<Record<string, SlotEntry[]>>({});
export const actionRegistry = reactive<Record<string, any[]>>({});

export function registerSlot(
  name: string,
  component: Component,
  order?: number,
  props?: (() => Record<string, any>) | Record<string, any>,
  events?: Record<string, (...args: any[]) => void>
): void {
  if (!slotRegistry[name]) slotRegistry[name] = [];
  
  // Prevent duplicate registration
  if (slotRegistry[name].some(e => e.component === component)) return;

  const entries = [...slotRegistry[name], { component, order, props, events }];
  entries.sort((a, b) => (a.order ?? 0) - (b.order ?? 0));
  slotRegistry[name] = entries;
}

export function getSlotComponents(name: string): SlotEntry[] {
  return slotRegistry[name] || [];
}

export function registerAction(name: string, action: any): void {
  if (!actionRegistry[name]) actionRegistry[name] = [];
  
  // Prevent duplicate registration
  if (actionRegistry[name].includes(action)) return;

  actionRegistry[name].push(action);
}

export function getActions(name: string): any[] {
  return actionRegistry[name] || [];
}
