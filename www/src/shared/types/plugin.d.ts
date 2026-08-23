export interface IPluginPage {
  slugName: string;
  name: string;
  icon: string;
  type: "full" | "tab" | "widget" | "modal";
  route: string;
  component: string;
  navPosition: "sidebar" | "topbar" | "settings" | "user-menu" | "none" | "sidebar-footer";
  navOrder: number;
  sidebarComponent?: string;
}
