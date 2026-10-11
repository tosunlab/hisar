import { reactive } from "vue";
import { api, type ApprovalEvent } from "./api";

// approvalStore drives the separate approval window; requests are answered first come, first served.
export const approvalStore = reactive({
  allApproval: [] as ApprovalEvent[],
  failureMessage: "",
  isAuthorizing: false,
  isRemembered: true,
  isPasswordChosen: false,
  isExpanded: false,
});

function resetCurrentApproval(): void {
  approvalStore.failureMessage = "";
  approvalStore.isAuthorizing = false;
  approvalStore.isRemembered = true;
  approvalStore.isPasswordChosen = false;
  approvalStore.isExpanded = false;
}

function isCurrent(requestID: string): boolean {
  return approvalStore.allApproval[0]?.requestID === requestID;
}

function getRememberedAnswer(requestID: string): boolean {
  const approval = approvalStore.allApproval.find((pending) => pending.requestID === requestID);
  return approvalStore.isRemembered && approval?.isRememberable === true;
}

export function addApproval(approval: ApprovalEvent): void {
  if (!approvalStore.allApproval.some((pending) => pending.requestID === approval.requestID)) {
    approvalStore.allApproval.push(approval);
  }
}

export function removeApproval(requestID: string): void {
  const wasCurrent = isCurrent(requestID);
  approvalStore.allApproval = approvalStore.allApproval.filter((pending) => pending.requestID !== requestID);
  if (wasCurrent) {
    resetCurrentApproval();
  }
}

// failApproval keeps the request open; a Touch ID check cancelled for the master password is not a failure.
export function failApproval(requestID: string, message: string): void {
  if (!isCurrent(requestID)) {
    return;
  }
  approvalStore.isAuthorizing = false;
  if (!(approvalStore.isPasswordChosen && message.startsWith("Touch ID"))) {
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
    await api.respondSSHApproval(requestID, true, getRememberedAnswer(requestID));
  } catch {
    failApproval(requestID, "Hisar could not answer the request.");
  }
}

// denyApproval also ends a Touch ID check that is waiting for the sensor.
export async function denyApproval(requestID: string): Promise<void> {
  removeApproval(requestID);
  await api.respondSSHApproval(requestID, false, false).catch(() => undefined);
}

// authorizeWithPassword unlocks a locked Hisar, or approves with the master password instead of Touch ID; a wrong password comes back as a failure event.
export async function authorizeWithPassword(requestID: string, password: string): Promise<void> {
  approvalStore.isAuthorizing = true;
  approvalStore.failureMessage = "";
  try {
    await api.respondApprovalWithPassword(requestID, password, getRememberedAnswer(requestID));
  } catch {
    failApproval(requestID, "Hisar could not answer the request.");
  }
}

export async function setRemembered(requestID: string, isRemembered: boolean): Promise<void> {
  approvalStore.isRemembered = isRemembered;
  await api.setSSHApprovalRemembered(requestID, isRemembered).catch(() => undefined);
}

// choosePassword stops Touch ID and shows the master password field; the request stays open.
export async function choosePassword(): Promise<void> {
  const wasAuthorizing = approvalStore.isAuthorizing;
  approvalStore.isPasswordChosen = true;
  approvalStore.isAuthorizing = false;
  approvalStore.failureMessage = "";
  if (wasAuthorizing) {
    await api.cancelApprovalTouchID().catch(() => undefined);
  }
}
