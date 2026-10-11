<script setup lang="ts">
import { computed, ref } from "vue";
import type { PendingRecovery, VaultSummary } from "../api";
import { completeAccountRecovery, enableRecoveryForAllVault, getFirstName, setUpRecovery, store } from "../store";
import { formatSubmittedTime, formatVaultCount } from "../util";
import AppIcon from "./AppIcon.vue";
import MemberAvatar from "./MemberAvatar.vue";

interface CompletedRecovery {
  accountId: string;
  firstName: string;
  allUnrecoverableVaultName: string[];
}

const isConfirmingSetUp = ref(false);
const allFailedVaultName = ref<string[]>([]);
const completingAccountID = ref("");
const completedRecovery = ref<CompletedRecovery | null>(null);

const recoveryStatus = computed(() => store.recoveryStatus);
const allOwnedVault = computed(() => store.allVault.filter((vault) => vault.role === "owner"));
const hasVaultToEnable = computed(() => allOwnedVault.value.some((vault) => !vault.recoverable && !vault.recoveryOptOut));
const creatorName = computed(() => recoveryStatus.value?.createdByName || "your admin");

const description = computed(() => {
  const status = recoveryStatus.value;
  if (!status) {
    return "";
  }
  if (!status.isSetUp) {
    return status.isAdmin
      ? "If someone forgets their master password or loses their Secret Key, their vaults are lost. Set up recovery so you can help them back in."
      : "If you forget your master password or lose your Secret Key, your vaults cannot be recovered. Ask your admin to set up recovery.";
  }
  if (status.isCreatedByCurrentUser) {
    return "You can help people who forgot their master password or lost their Secret Key. Only you can complete recoveries, so keep your own master password and Secret Key safe.";
  }
  if (status.isAdmin) {
    return `Set up by ${creatorName.value}. You can start recoveries, but only ${creatorName.value} can complete them.`;
  }
  return `${creatorName.value} can help you back in if you forget your master password or lose your Secret Key.`;
});

function describeVaultRecovery(vault: VaultSummary): { label: string; tone: string } {
  if (vault.recoverable) {
    return { label: "Recoverable", tone: "success" };
  }
  if (vault.recoveryOptOut) {
    return { label: "Recovery off", tone: "warning" };
  }
  return { label: "Not recoverable yet", tone: "" };
}

function describeVaultCounts(pending: PendingRecovery): string {
  const totalCount = pending.recoverableVaultCount + pending.unrecoverableVaultCount;
  if (pending.unrecoverableVaultCount === 0) {
    return totalCount === 1 ? "Their vault can be recovered" : `All ${totalCount} of their vaults can be recovered`;
  }
  return `${pending.recoverableVaultCount} of ${formatVaultCount(totalCount)} can be recovered`;
}

async function confirmSetUp(): Promise<void> {
  const allFailed = await setUpRecovery();
  if (allFailed) {
    allFailedVaultName.value = allFailed;
    isConfirmingSetUp.value = false;
  }
}

async function submitEnableAll(): Promise<void> {
  const allFailed = await enableRecoveryForAllVault();
  if (allFailed) {
    allFailedVaultName.value = allFailed;
  }
}

async function confirmComplete(pending: PendingRecovery): Promise<void> {
  const allUnrecoverableVaultName = await completeAccountRecovery(pending);
  completingAccountID.value = "";
  if (allUnrecoverableVaultName) {
    completedRecovery.value = { accountId: pending.accountId, firstName: getFirstName(pending), allUnrecoverableVaultName };
  }
}
</script>

<template>
  <section data-testid="recovery-section">
    <p class="section-label">Account recovery</p>
    <p v-if="!recoveryStatus" class="muted empty-note">{{ store.isBusy ? "Checking account recovery…" : "Account recovery status is not available." }}</p>
    <template v-else>
      <div class="setting">
        <span class="setting-icon"><AppIcon name="shield" /></span>
        <div class="setting-text">
          <strong class="title-line">
            <span class="title-text">Account recovery</span>
            <span class="tag" :class="{ success: recoveryStatus.isSetUp }" data-testid="recovery-state">{{ recoveryStatus.isSetUp ? "On" : "Off" }}</span>
          </strong>
          <p class="muted" data-testid="recovery-description">{{ description }}</p>
        </div>
        <button
          v-if="recoveryStatus.isAdmin && !recoveryStatus.isSetUp && !isConfirmingSetUp"
          class="primary"
          data-testid="start-recovery-setup"
          @click="isConfirmingSetUp = true"
        >
          Set up
        </button>
      </div>

      <div v-if="isConfirmingSetUp" class="recovery-note" data-testid="recovery-setup-confirmation">
        <AppIcon name="shield" class="note-icon" />
        <div class="note-body">
          <p>
            Your devices get a recovery key that can open every recoverable vault, so everyone whose vaults are recoverable trusts you with their data. Only you can complete
            recoveries: keep your own master password and Secret Key safe.
          </p>
          <p class="muted">Vaults are recoverable by default. Members can turn recovery off for their personal vault, which is then lost if they lose access.</p>
          <div class="row note-actions">
            <button class="ghost" :disabled="store.isBusy" @click="isConfirmingSetUp = false">Cancel</button>
            <button class="primary" data-testid="confirm-recovery-setup" :disabled="store.isBusy" @click="confirmSetUp">Set up recovery</button>
          </div>
        </div>
      </div>

      <p v-if="allFailedVaultName.length > 0" class="recovery-warning" data-testid="recovery-failed-vaults">
        <AppIcon name="warning" :size="14" />
        <span>Could not turn on recovery for {{ allFailedVaultName.join(", ") }}. Try again later.</span>
      </p>

      <div v-if="recoveryStatus.isSetUp && allOwnedVault.length > 0" class="vault-list" data-testid="recovery-vault-list">
        <p class="list-label">Your vaults</p>
        <div v-for="vault in allOwnedVault" :key="vault.id" class="vault-row" :data-testid="`vault-recovery-${vault.id}`">
          <AppIcon :name="vault.type === 'personal' ? 'user' : 'users'" :size="14" class="vault-icon" />
          <span class="vault-name">{{ vault.name }}</span>
          <span class="tag" :class="describeVaultRecovery(vault).tone">{{ describeVaultRecovery(vault).label }}</span>
        </div>
        <div v-if="hasVaultToEnable" class="row enable-all">
          <p class="muted">Some of your vaults were created before recovery was set up.</p>
          <button data-testid="enable-recovery-all" :disabled="store.isBusy" @click="submitEnableAll">Make recoverable</button>
        </div>
      </div>

      <template v-if="recoveryStatus.isAdmin && recoveryStatus.isSetUp">
        <p class="list-label pending-label">Pending recoveries</p>
        <p v-if="store.allPendingRecovery.length === 0 && !completedRecovery" class="muted empty-note" data-testid="no-pending-recovery">
          No pending recoveries. Start one from a person under People above; it shows up here once they open their link.
        </p>
        <div v-if="completedRecovery" class="recovery-result" data-testid="recovery-result">
          <p>
            <AppIcon name="check" :size="14" class="result-icon" />
            {{ completedRecovery.firstName }} can now sign in with their new Secret Key and master password.
          </p>
          <template v-if="completedRecovery.allUnrecoverableVaultName.length > 0">
            <p class="warning-text">These vaults could not be recovered, so {{ completedRecovery.firstName }} lost access to them:</p>
            <ul data-testid="unrecoverable-vaults">
              <li v-for="vaultName in completedRecovery.allUnrecoverableVaultName" :key="vaultName">{{ vaultName }}</li>
            </ul>
          </template>
          <button class="ghost" @click="completedRecovery = null">Done</button>
        </div>
        <template v-for="pending in store.allPendingRecovery" :key="pending.accountId">
          <div class="setting" :data-testid="`pending-recovery-${pending.accountId}`">
            <MemberAvatar :name="pending.name || pending.email" />
            <div class="setting-text">
              <strong class="title-line"><span class="title-text">{{ pending.name || pending.email }}</span></strong>
              <p class="muted">{{ pending.email }} · Asked {{ formatSubmittedTime(pending.submittedAt) }}</p>
              <p :class="pending.unrecoverableVaultCount > 0 ? 'warning-text' : 'muted'">{{ describeVaultCounts(pending) }}</p>
            </div>
            <button
              v-if="completingAccountID !== pending.accountId"
              class="primary"
              :disabled="!recoveryStatus.isCreatedByCurrentUser"
              :title="recoveryStatus.isCreatedByCurrentUser ? 'Complete this recovery' : `Only ${creatorName} can complete recoveries`"
              :data-testid="`complete-recovery-${pending.accountId}`"
              @click="completingAccountID = pending.accountId"
            >
              Complete
            </button>
          </div>
          <div v-if="completingAccountID === pending.accountId" class="recovery-note" data-testid="complete-recovery-confirmation">
            <AppIcon name="refresh" class="note-icon" />
            <div class="note-body">
              <p>
                {{ getFirstName(pending) }}'s devices are signed out and their old Secret Key stops working. They sign in with the new Secret Key and
                master password they chose.
              </p>
              <p v-if="pending.unrecoverableVaultCount > 0" class="warning-text">
                {{ formatVaultCount(pending.unrecoverableVaultCount) }} cannot be recovered; {{ getFirstName(pending) }} loses access to
                {{ pending.unrecoverableVaultCount === 1 ? "it" : "them" }}.
              </p>
              <div class="row note-actions">
                <button class="ghost" :disabled="store.isBusy" @click="completingAccountID = ''">Cancel</button>
                <button class="primary" data-testid="confirm-complete-recovery" :disabled="store.isBusy" @click="confirmComplete(pending)">
                  Complete recovery
                </button>
              </div>
            </div>
          </div>
        </template>
      </template>
    </template>
  </section>
</template>

<style scoped>
.recovery-note {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin: 0 0 12px 44px;
  padding: 10px 12px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius);
  background: var(--color-surface);
  font-size: 12px;
}

.note-icon {
  flex: none;
  margin-top: 1px;
  color: var(--color-text-muted);
}

.note-body {
  flex: 1;
}

.note-body p {
  margin: 0 0 6px;
}

.note-actions {
  justify-content: flex-end;
  margin-top: 8px;
}

.recovery-warning {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin: 0 0 12px 44px;
  font-size: 12px;
  color: var(--color-warning);
}

.list-label {
  margin: 10px 0 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-secondary);
}

.vault-list {
  padding: 0 0 6px 44px;
}

.vault-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 0;
  font-size: 12px;
}

.vault-icon {
  color: var(--color-text-muted);
}

.vault-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.enable-all {
  margin-top: 6px;
}

.enable-all p {
  flex: 1;
  margin: 0;
  font-size: 12px;
}

.pending-label {
  margin-top: 14px;
}

.recovery-result {
  margin: 0 0 10px;
  padding: 10px 12px;
  border: 1px solid #1f4a32;
  border-radius: var(--radius);
  background: #0f1d16;
  font-size: 12px;
}

.recovery-result p {
  margin: 0 0 6px;
}

.recovery-result ul {
  margin: 0 0 8px;
  padding-left: 18px;
  color: var(--color-text-secondary);
}

.result-icon {
  margin-right: 4px;
  vertical-align: -2px;
  color: var(--color-success);
}
</style>
