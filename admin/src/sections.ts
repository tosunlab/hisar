import type { Component } from "vue";
import InvitesView from "./sections/invites/InvitesView.vue";
import OverviewView from "./sections/overview/OverviewView.vue";
import SettingsView from "./sections/settings/SettingsView.vue";

export interface Section {
  id: string;
  title: string;
  path: string;
  component: Component;
}

const overview: Section = { id: "overview", title: "Overview", path: "/admin", component: OverviewView };

export const sections: readonly Section[] = [
  overview,
  { id: "invites", title: "Invites", path: "/admin/invites", component: InvitesView },
  { id: "settings", title: "Settings", path: "/admin/settings", component: SettingsView },
];

export function findSection(pathname: string): Section {
  const path = pathname.replace(/\/+$/, "");
  const allOwner = sections.filter((section) => path === section.path || path.startsWith(`${section.path}/`));
  return allOwner.reduce((longest, section) => (section.path.length > longest.path.length ? section : longest), overview);
}
