<script setup lang="ts">
import { computed, ref } from "vue";
import type { SentShareInvitation } from "../api";
import { acceptShareInvitation, cancelShareInvitation, declineShareInvitation, invitePerson, store } from "../store";
import { formatRole } from "../util";
import AppIcon from "./AppIcon.vue";
import MemberAvatar from "./MemberAvatar.vue";

const isInviting = ref(false);
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
  return invitation.state === "accepted" ? "Accepted · finishing on your next sync" : "Waiting for them to accept";
}

function describeInviter(name: string, email: string): string {
  return name && name !== email ? name : email;
}
</script>

<template>
  <section data-testid="people-section">
    <p class="section-label">People</p>

    <div v-if="store.shareInvitations.received.length > 0" class="invitation-group" data-testid="received-invitations">
      <p class="group-label">Invitations for you</p>
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

    <p v-if="store.allPerson.length === 0" class="muted empty-note">{{ store.isBusy ? "Loading people…" : "No people to show." }}</p>
    <template v-for="person in store.allPerson" :key="person.accountId">
      <div class="setting" :data-testid="`person-${person.accountId}`">
        <MemberAvatar :name="person.name || person.email" />
        <div class="setting-text">
          <strong class="title-line">
            <span class="title-text">{{ person.name || person.email }}</span>
            <span v-if="person.isCurrentUser" class="tag accent">you</span>
          </strong>
          <p class="muted">{{ person.email }}</p>
          <p v-if="person.isKeyChanged" class="warning-text">Their key changed since you last shared with them. Compare security codes before you share more.</p>
        </div>
        <div class="security-code" :title="person.isCurrentUser ? 'Your security code' : 'Compare this code with them over a channel you trust'">
          <span class="code-label">Security code</span>
          <span class="monospace" data-testid="person-security-code">{{ person.securityCode }}</span>
        </div>
      </div>
    </template>
    <p v-if="isAlone" class="muted hint" data-testid="people-empty-hint">
      People appear here once you share a vault with them. Open a shared vault and choose Share to invite someone by email.
    </p>

    <p v-if="store.shareInvitations.completionError" class="warning-text completion-error" data-testid="share-completion-error">
      Could not finish sharing: {{ store.shareInvitations.completionError }}
    </p>
    <div v-if="store.shareInvitations.sent.length > 0" class="invitation-group" data-testid="sent-invitations">
      <p class="group-label">Invitations you sent</p>
      <div v-for="invitation in store.shareInvitations.sent" :key="invitation.id" class="setting" :data-testid="`sent-invitation-${invitation.id}`">
        <span class="setting-icon"><AppIcon name="mail" /></span>
        <div class="setting-text">
          <strong class="title-line">
            <span class="title-text">{{ invitation.email }}</span>
            <span class="tag" :class="{ success: invitation.state === 'accepted' }">{{ formatRole(invitation.role) }}</span>
          </strong>
          <p class="muted">{{ invitation.vaultName || "A vault you own" }} · {{ describeSentState(invitation) }}</p>
        </div>
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

    <div class="setting">
      <span class="setting-icon"><AppIcon name="users" /></span>
      <div class="setting-text">
        <strong>Invite someone to Hisar</strong>
        <p class="muted">The server emails them a link to create an account. Share a vault with them from the vault's Share panel.</p>
      </div>
      <button v-if="!isInviting" data-testid="start-invite" @click="isInviting = true">Invite</button>
    </div>
    <form v-if="isInviting" class="inline-form" data-testid="invite-form" @submit.prevent="submitInvite">
      <input v-model="inviteEmail" name="inviteEmail" type="email" aria-label="Email" placeholder="Email address" autocomplete="off" autofocus required />
      <button type="button" class="ghost" @click="isInviting = false">Cancel</button>
      <button class="primary" type="submit" data-testid="send-invite" :disabled="store.isBusy || !inviteEmail.trim()">Send invite</button>
    </form>
  </section>
</template>

<style scoped>
section {
  margin-bottom: 18px;
}

.invitation-group {
  margin-bottom: 8px;
}

.group-label {
  margin: 10px 0 2px;
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-secondary);
}

.hint,
.completion-error {
  margin: 4px 0 10px 44px;
  font-size: 12px;
}

.inline-form {
  display: flex;
  gap: 8px;
  padding: 0 0 12px 44px;
}

.inline-form button {
  flex: none;
  white-space: nowrap;
}

.security-code {
  display: flex;
  flex: none;
  flex-direction: column;
  align-items: flex-end;
  font-size: 12px;
  color: var(--color-text-secondary);
  user-select: text;
  -webkit-user-select: text;
}

.code-label {
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--color-text-muted);
}
</style>
