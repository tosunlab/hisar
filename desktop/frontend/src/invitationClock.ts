import { onBeforeUnmount, onMounted, ref } from "vue";

export function useInvitationClock() {
  const now = ref(new Date());
  let timer: number | undefined;
  onMounted(() => { timer = window.setInterval(() => { now.value = new Date(); }, 1000); });
  onBeforeUnmount(() => window.clearInterval(timer));
  return now;
}
