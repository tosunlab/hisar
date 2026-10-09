import { createApp } from "vue";
import { readSessionID, supportsVisibilityTracking } from "../inline-menu/api";
import { connectPromptChannel } from "./api";
import PasskeyPrompt from "./PasskeyPrompt.vue";

createApp(PasskeyPrompt, { channel: connectPromptChannel(readSessionID(window.location)), isVisibilityTracked: supportsVisibilityTracking() }).mount("#app");
