import { createApp } from "vue";
import App from "./App.vue";
import { api } from "./api";
import "./style.css";

// VITE_PREVIEW is a build-time constant: production builds drop these dynamic imports entirely.
async function startApp(): Promise<void> {
  if (import.meta.env.VITE_PREVIEW === "true") {
    const { previewApi } = await import("./preview");
    Object.assign(api, previewApi);
  }
  createApp(App).mount("#app");
  if (import.meta.env.VITE_PREVIEW === "true") {
    const { applyPreviewParameter } = await import("./preview-bootstrap");
    await applyPreviewParameter();
  }
}

startApp();
