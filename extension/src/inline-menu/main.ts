import { createApp } from "vue";
import { readSessionID, supportsVisibilityTracking } from "./api";
import InlineMenu from "./InlineMenu.vue";

createApp(InlineMenu, { sessionID: readSessionID(window.location), isVisibilityTracked: supportsVisibilityTracking() }).mount("#app");
