import type { Component } from "vue";
import SettingsView from "./sections/settings/SettingsView.vue";
import HomeView from "./views/HomeView.vue";

export interface Section {
  id: string;
  title: string;
  path: string;
  component: Component;
}

const overview: Section = { id: "overview", title: "Overview", path: "/admin", component: HomeView };

export const sections: readonly Section[] = [overview, { id: "settings", title: "Settings", path: "/admin/settings", component: SettingsView }];

export function findSection(pathname: string): Section {
  const path = pathname.replace(/\/+$/, "");
  const allOwner = sections.filter((section) => path === section.path || path.startsWith(`${section.path}/`));
  return allOwner.reduce((longest, section) => (section.path.length > longest.path.length ? section : longest), overview);
}
