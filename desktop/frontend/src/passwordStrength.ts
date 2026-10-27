import { computed, shallowRef, watch, type ComputedRef } from "vue";
import { api, type MasterPasswordFeedback, type PasswordStrength } from "./api";

interface Rating<T> {
  value: ComputedRef<T>;
  // isCurrent is false while the backend still rates the password just typed; the forms keep submitting off until then.
  isCurrent: ComputedRef<boolean>;
}

function useRating<T>(getPassword: () => string, rate: (password: string) => Promise<T>, unrated: T): Rating<T> {
  const latest = shallowRef<{ password: string; value: T } | null>(null);
  watch(
    getPassword,
    async (password) => {
      const value = await rate(password);
      if (getPassword() === password) {
        latest.value = { password, value };
      }
    },
    { immediate: true },
  );
  return { value: computed(() => latest.value?.value ?? unrated), isCurrent: computed(() => latest.value?.password === getPassword()) };
}

export function usePasswordStrength(getPassword: () => string): ComputedRef<PasswordStrength> {
  return useRating(getPassword, (password) => api.ratePassword(password), { level: "", label: "", bits: 0 }).value;
}

export function useMasterPasswordFeedback(getPassword: () => string): Rating<MasterPasswordFeedback> {
  return useRating(getPassword, (password) => api.checkMasterPassword(password), { level: "", label: "", bits: 0, message: "", isBlocking: false });
}
