import { ref } from "vue";

// currentPath is the dashboard page the address bar shows; App.vue follows the browser's back and forward buttons.
export const currentPath = ref(window.location.pathname);

// navigate shows another page of the dashboard without a reload.
export function navigate(path: string): void {
  window.history.pushState(null, "", path);
  currentPath.value = path;
}
