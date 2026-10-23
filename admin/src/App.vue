<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { auth, getErrorMessage, signOut } from "./api";
import { currentPath, navigate } from "./navigation";
import { findSection, sections } from "./sections";
import SetupView from "./views/SetupView.vue";
import SignInView from "./views/SignInView.vue";
import { listRecovery } from "./bridge/recoveryStorage";

const currentSection = computed(() => findSection(currentPath.value));
const isSetupPage = computed(() => currentPath.value.replace(/\/+$/, "") === "/admin/setup");
const isSigningOut = ref(false);
const signOutError = ref("");
const recoveryError = ref("");

// leaveSetup opens the dashboard, signed in or at sign-in, and drops the setup invite from the address bar.
function leaveSetup(): void {
  window.history.replaceState(null, "", "/admin");
  currentPath.value = "/admin";
}

function followHistory(): void {
  currentPath.value = window.location.pathname;
}

onMounted(async () => {
  window.addEventListener("popstate", followHistory);
  if (!isSetupPage.value) {
    try {
      if ((await listRecovery()).length > 0) navigate("/admin/setup");
    } catch (error) {
      recoveryError.value = getErrorMessage(error);
    }
  }
});
onUnmounted(() => window.removeEventListener("popstate", followHistory));

async function submitSignOut(): Promise<void> {
  isSigningOut.value = true;
  signOutError.value = "";
  try {
    await signOut();
  } catch (error) {
    signOutError.value = getErrorMessage(error);
  } finally {
    isSigningOut.value = false;
  }
}
</script>

<template>
  <p v-if="recoveryError" class="error" role="alert">{{ recoveryError }}</p>
  <SetupView v-if="isSetupPage" @leave="leaveSetup" />
  <SignInView v-else-if="!auth.session" />
  <div v-else class="shell">
    <aside class="sidebar">
      <p class="product">Hisar · Admin dashboard</p>
      <nav aria-label="Sections">
        <a
          v-for="section in sections"
          :key="section.id"
          :href="section.path"
          :aria-current="section.id === currentSection.id ? 'page' : undefined"
          @click.prevent="navigate(section.path)"
          >{{ section.title }}</a
        >
      </nav>
      <div class="account">
        <p class="account-name">{{ auth.session.name }}</p>
        <button type="button" :disabled="isSigningOut" @click="submitSignOut">{{ isSigningOut ? "Signing out…" : "Sign out" }}</button>
        <p v-if="signOutError" class="error" role="alert">{{ signOutError }}</p>
      </div>
    </aside>
    <main class="content">
      <component :is="currentSection.component" :key="currentSection.id" />
    </main>
  </div>
</template>

<style scoped>
.shell {
  display: grid;
  grid-template-columns: 240px 1fr;
  min-height: 100%;
}

.sidebar {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 20px 14px;
  border-right: 1px solid var(--color-border);
  background: var(--color-sidebar);
}

.product {
  margin: 0 8px;
  color: var(--color-text-muted);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
}

nav {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 2px;
}

nav a {
  padding: 7px 10px;
  border-radius: var(--radius);
  color: var(--color-text-secondary);
  text-decoration: none;
}

nav a:hover {
  background: var(--color-hover);
}

nav a[aria-current="page"] {
  background: var(--color-selected);
  color: var(--color-text);
  font-weight: 600;
}

.account {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 0 8px;
}

.account-name {
  margin: 0;
  overflow: hidden;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.content {
  padding: 32px 40px;
  overflow-y: auto;
}

@media (max-width: 720px) {
  .shell {
    grid-template-columns: 1fr;
  }

  .sidebar {
    border-right: none;
    border-bottom: 1px solid var(--color-border);
  }
}
</style>
