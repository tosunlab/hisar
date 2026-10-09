import { reactive } from "vue";
import { api, type ApprovalEvent } from "./api";

// approvalStore drives the separate SSH approval window; requests are answered first come, first served.
export const approvalStore = reactive({
  allApproval: [] as ApprovalEvent[],
  failureMessage: "",
  isAuthorizing: false,
  isForAllApplication: false,
});

function resetCurrentApproval(): void {
  approvalStore.failureMessage = "";
  approvalStore.isAuthorizing = false;
  approvalStore.isForAllApplication = false;
}

export function addApproval(approval: ApprovalEvent): void {
  if (!approvalStore.allApproval.some((pending) => pending.requestID === approval.requestID)) {
    approvalStore.allApproval.push(approval);
  }
}

export function removeApproval(requestID: string): void {
  const wasCurrent = approvalStore.allApproval[0]?.requestID === requestID;
  approvalStore.allApproval = approvalStore.allApproval.filter((pending) => pending.requestID !== requestID);
  if (wasCurrent) {
    resetCurrentApproval();
  }
}

export function failApproval(requestID: string, message: string): void {
  if (approvalStore.allApproval[0]?.requestID === requestID) {
    approvalStore.isAuthorizing = false;
    approvalStore.failureMessage = message;
  }
}

export async function loadAllPendingApproval(): Promise<void> {
  try {
    (await api.listAllPendingSSHApproval()).forEach(addApproval);
  } catch {
    return;
  }
}

// authorizeApproval waits for the backend: with Touch ID the request closes, or fails and can be tried again.
export async function authorizeApproval(requestID: string): Promise<void> {
  approvalStore.isAuthorizing = true;
  approvalStore.failureMessage = "";
  try {
    await api.respondSSHApproval(requestID, true, approvalStore.isForAllApplication);
  } catch {
    failApproval(requestID, "Hisar could not answer the request.");
  }
}

export async function denyApproval(requestID: string): Promise<void> {
  removeApproval(requestID);
  await api.respondSSHApproval(requestID, false, false).catch(() => undefined);
}
