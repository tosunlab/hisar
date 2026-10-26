import { createApp } from "vue";
import App from "./App.vue";
import SSHApprovalWindow from "./components/SSHApprovalWindow.vue";
import SettingsWindow from "./SettingsWindow.vue";
import { api } from "./api";
import { applyCachedTheme } from "./theme";
import { openExternalLink } from "./externalLink";
import "./themes.css";
import "./style.css";

// VITE_PREVIEW is a build-time constant: production builds drop these dynamic imports entirely.
async function startApp(): Promise<void> {
  if (import.meta.env.VITE_PREVIEW === "true") {
    const { previewApi } = await import("./preview");
    Object.assign(api, previewApi);
  }
  applyCachedTheme();
  document.addEventListener("click", openExternalLink);
  const windowName = new URLSearchParams(window.location.search).get("window");
  createApp(windowName === "ssh-approval" ? SSHApprovalWindow : windowName === "settings" ? SettingsWindow : App).mount("#app");
  if (import.meta.env.VITE_PREVIEW === "true") {
    const { applyPreviewParameter } = await import("./preview-bootstrap");
    await applyPreviewParameter();
  }
}

startApp();
