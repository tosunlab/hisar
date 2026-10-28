import { reactive } from "vue";
import { api, type ApprovalEvent } from "./api";

// approvalStore drives the separate approval window; requests are answered first come, first served.
export const approvalStore = reactive({
  allApproval: [] as ApprovalEvent[],
  failureMessage: "",
  isAuthorizing: false,
  isRemembered: true,
  grantDuration: "once",
  loginIndex: 0,
  isPasswordChosen: false,
  isExpanded: false,
});

function applyAgentChoice(approval: ApprovalEvent | undefined): void {
  approvalStore.grantDuration = approval?.grantDuration || "once";
  approvalStore.loginIndex = approval?.loginIndex ?? 0;
}

// resetCurrentApproval prepares the next request; agent requests start with the checkbox off.
function resetCurrentApproval(): void {
  approvalStore.failureMessage = "";
  approvalStore.isAuthorizing = false;
  approvalStore.isRemembered = approvalStore.allApproval[0]?.isRememberedByDefault ?? true;
  applyAgentChoice(approvalStore.allApproval[0]);
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
  const existing = approvalStore.allApproval.find((pending) => pending.requestID === approval.requestID);
  if (existing) {
    Object.assign(existing, approval);
    return;
  }
  approvalStore.allApproval.push(approval);
  if (approvalStore.allApproval.length === 1) {
    approvalStore.isRemembered = approval.isRememberedByDefault ?? true;
    applyAgentChoice(approval);
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

// setGrantDuration and setSignInLogin tell the backend at once: a Touch ID check in progress starts again with a reason
// that names the new choice.
export async function setGrantDuration(requestID: string, duration: string): Promise<void> {
  approvalStore.grantDuration = duration;
  await api.setSSHApprovalGrantDuration(requestID, duration).catch(() => undefined);
}

export async function setSignInLogin(requestID: string, loginIndex: number): Promise<void> {
  approvalStore.loginIndex = loginIndex;
  await api.setSSHApprovalLogin(requestID, loginIndex).catch(() => undefined);
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
