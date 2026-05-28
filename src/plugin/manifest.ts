import type { Component } from "vue";

export type PluginCapability = "route" | "slot" | "settings" | "event";

export interface PluginRoute {
  path: string;
  name: string;
  component: Component | (() => Promise<Component>);
}

export interface PluginSlot {
  name: string;
  component: Component;
  order?: number;
}

export interface PluginContext {
  router: any;
  Layout: Component;
  api: any;
  registerComponent: (name: string, component: Component) => void;
  registerAction: (slot: string, action: any) => void;
  registerSlotComponent: (slot: string, component: Component) => void;
  registerLoginButton: (component: Component) => void;
  emit: (event: string, ...args: any[]) => void;
  on: (event: string, callback: (...args: any[]) => void) => void;
}

export interface AbyssPluginManifest {
  id: string;
  slugName?: string;
  name: string;
  version: string;
  sdkVersion: string;
  capabilities: PluginCapability[];
  settings?: PluginSettingMetadata[];
  globalSettings?: PluginSettingMetadata[];
  i18n?: Record<string, Record<string, any>>;
  routes?: PluginRoute[];
  slots?: PluginSlot[];
  register(ctx: PluginContext): void;
  mount?(): void | Promise<void>;
  unmount?(): void | Promise<void>;
}

export interface PluginSettingMetadata {
  component: string;
  label: string;
  icon?: string;
  slug?: string;
  category?: string;
}
