<script setup lang="ts">
import { computed, ref } from "vue";
import type { SentShareInvitation } from "../api";
import { acceptShareInvitation, cancelShareInvitation, declineShareInvitation, invitePerson, store } from "../store";
import { useSettingsRowVisibility } from "../settingsSections";
import { formatRole } from "../util";
import AppIcon from "./AppIcon.vue";
import ShareRecipientConfirmation from "./ShareRecipientConfirmation.vue";
import MemberAvatar from "./MemberAvatar.vue";

const isInviting = defineModel<boolean>("isInviting", { default: false });
const isRowVisible = useSettingsRowVisibility();
const inviteEmail = ref("");
const cancellingInvitationID = ref("");
const isAlone = computed(() => store.allPerson.length > 0 && store.allPerson.every((person) => person.isCurrentUser));

async function submitInvite(): Promise<void> {
  if (await invitePerson(inviteEmail.value.trim())) {
    inviteEmail.value = "";
    isInviting.value = false;
  }
}

async function confirmCancel(invitation: SentShareInvitation): Promise<void> {
  if (await cancelShareInvitation(invitation)) {
    cancellingInvitationID.value = "";
  }
}

function describeSentState(invitation: SentShareInvitation): string {
  if (invitation.requiresConfirmation) return "Accepted · Review recipient identity";
  return invitation.state === "accepted" ? "Accepted · Hisar is finishing sharing" : "Waiting for them to accept";
}

function describeInviter(name: string, email: string): string {
  return name && name !== email ? name : email;
}
</script>

<template>
  <section data-testid="people-section">
    <div v-if="isInviting" class="settings-block">
      <p class="section-label">Invite to Hisar</p>
      <div class="settings-group">
        <p class="muted invite-explanation" data-testid="invite-explanation">
          Creates a Hisar account: the server emails them a sign-up link. To give them a vault, share it from the vault's Share panel.
        </p>
        <form class="settings-group-form" data-testid="invite-form" @submit.prevent="submitInvite">
          <input v-model="inviteEmail" name="inviteEmail" type="email" aria-label="Email" placeholder="Email address" autocomplete="off" autofocus required />
          <button type="button" class="ghost" @click="isInviting = false">Cancel</button>
          <button class="primary" type="submit" data-testid="send-invite" :disabled="store.isBusy || !inviteEmail.trim()">Send invite</button>
        </form>
      </div>
    </div>

    <div v-if="store.shareInvitations.received.length > 0" v-show="isRowVisible('received-invitations')" class="settings-block" data-testid="received-invitations">
      <p class="section-label">Invitations for you</p>
      <div class="settings-group">
        <div v-for="invitation in store.shareInvitations.received" :key="invitation.id" class="setting" :data-testid="`received-invitation-${invitation.id}`">
          <MemberAvatar :name="describeInviter(invitation.invitedByName, invitation.invitedByEmail)" />
          <div class="setting-text">
            <strong class="title-line">
              <span class="title-text">{{ describeInviter(invitation.invitedByName, invitation.invitedByEmail) }} shared a vault with you</span>
              <span class="tag accent">{{ formatRole(invitation.role) }}</span>
            </strong>
            <p class="muted">{{ invitation.invitedByEmail }} · You get access after you accept and their Hisar finishes sharing.</p>
          </div>
          <button class="ghost" :disabled="store.isBusy" :data-testid="`decline-invitation-${invitation.id}`" @click="declineShareInvitation(invitation)">Decline</button>
          <button class="primary" :disabled="store.isBusy" :data-testid="`accept-invitation-${invitation.id}`" @click="acceptShareInvitation(invitation)">Accept</button>
        </div>
      </div>
    </div>

    <div v-show="isRowVisible('people')" class="settings-block">
      <p class="section-label">People you share with</p>
      <p v-if="store.allPerson.length === 0" class="muted empty-note">{{ store.isBusy ? "Loading people…" : "No people to show." }}</p>
      <div v-else class="settings-group">
        <div v-for="person in store.allPerson" :key="person.accountId" class="setting" :data-testid="`person-${person.accountId}`">
          <MemberAvatar :name="person.name || person.email" />
          <div class="setting-text">
            <strong class="title-line">
              <span class="title-text">{{ person.name || person.email }}</span>
              <span v-if="person.isCurrentUser" class="tag accent">you</span>
            </strong>
            <p class="muted">{{ person.email }}</p>
            <p v-if="person.isKeyChanged" class="warning-text">Their key changed since you last shared with them. Compare security codes before you share more.</p>
          </div>
          <span
            class="security-code monospace"
            data-testid="person-security-code"
            :title="person.isCurrentUser ? 'Your security code' : 'Compare this code with them over a channel you trust'"
            :aria-label="`Security code ${person.securityCode}`"
            >{{ person.securityCode }}</span
          >
        </div>
      </div>
      <p v-if="isAlone" class="muted settings-note" data-testid="people-empty-hint">
        People appear here once you share a vault with them. Open a shared vault and choose Share to invite someone by email.
      </p>
      <p v-else-if="store.allPerson.length > 0" class="muted settings-note">Compare a security code with that person over a channel you trust.</p>
      <p v-if="store.shareInvitations.completionError" class="warning-text settings-note" data-testid="share-completion-error">
        Could not finish sharing: {{ store.shareInvitations.completionError }}
      </p>
    </div>

    <div v-if="store.shareInvitations.sent.length > 0" v-show="isRowVisible('sent-invitations')" class="settings-block" data-testid="sent-invitations">
      <p class="section-label">Invitations you sent</p>
      <div class="settings-group">
        <div v-for="invitation in store.shareInvitations.sent" :key="invitation.id" class="setting" :data-testid="`sent-invitation-${invitation.id}`">
          <span class="settings-row-icon"><AppIcon name="mail" :size="14" /></span>
          <div class="setting-text">
            <strong class="title-line">
              <span class="title-text">{{ invitation.email }}</span>
              <span class="tag" :class="{ success: invitation.state === 'accepted' }">{{ formatRole(invitation.role) }}</span>
            </strong>
            <p class="muted">{{ invitation.vaultName || "A vault you own" }} · {{ describeSentState(invitation) }}</p>
          </div>
          <ShareRecipientConfirmation :invitation="invitation" />
          <button
            v-if="cancellingInvitationID !== invitation.id"
            class="ghost"
            :data-testid="`cancel-invitation-${invitation.id}`"
            @click="cancellingInvitationID = invitation.id"
          >
            Cancel
          </button>
          <template v-else>
            <button class="ghost" @click="cancellingInvitationID = ''">Keep</button>
            <button class="danger-solid" data-testid="confirm-cancel-invitation" :disabled="store.isBusy" @click="confirmCancel(invitation)">Cancel invitation</button>
          </template>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.invite-explanation {
  margin: 0;
  padding: 10px 14px 8px;
  font-size: 12px;
}

.security-code {
  flex: none;
  font-size: 12px;
  color: var(--color-text-secondary);
  user-select: text;
  -webkit-user-select: text;
}
</style>
