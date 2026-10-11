<script setup lang="ts">
import { configureSSHConfig, removeSSHConfig, setCompactSSHApproval, setPhoneApproval, setSigningRemember, store } from "../store";
import { useSettingsRowVisibility } from "../settingsSections";

const isRowVisible = useSettingsRowVisibility();
const allSigningRememberOption = [
  { seconds: 0, label: "Always ask" },
  { seconds: 600, label: "Up to 10 minutes" },
  { seconds: 3600, label: "Up to 1 hour" },
];
</script>

<template>
  <section data-testid="ssh-agent-section">
    <div v-show="isRowVisible('ssh-agent')" class="settings-block">
      <p class="section-label">SSH agent</p>
      <div class="settings-group">
        <div class="setting">
          <div class="setting-text">
            <strong>Hisar SSH agent</strong>
            <p v-if="!store.sshAgentStatus" class="muted">Checking the SSH agent…</p>
            <p v-else-if="store.sshAgentStatus.isRunning" class="muted" data-testid="ssh-agent-state">
              <span class="state-dot running"></span>Running at <span class="monospace">{{ store.sshAgentStatus.socketPath }}</span>
            </p>
            <p v-else class="muted" data-testid="ssh-agent-state">
              <span class="state-dot stopped"></span>Not running<template v-if="store.sshAgentStatus.errorMessage">: {{ store.sshAgentStatus.errorMessage }}</template>
            </p>
          </div>
          <template v-if="store.sshAgentStatus">
            <button v-if="store.sshAgentStatus.isSSHConfigured" data-testid="remove-ssh-config" :disabled="store.isBusy" @click="removeSSHConfig">
              Remove from SSH config
            </button>
            <button v-else class="primary" data-testid="configure-ssh-config" :disabled="store.isBusy" @click="configureSSHConfig">Use Hisar for SSH</button>
          </template>
        </div>
        <div v-if="store.sshAgentStatus" class="settings-group-detail">
          <p v-if="store.sshAgentStatus.isSSHConfigured" class="muted">
            <span class="monospace">ssh</span> and <span class="monospace">git</span> use the SSH keys in your vaults while Hisar is unlocked.
          </p>
          <details v-else class="config-details">
            <summary>
              What “Use Hisar for SSH” adds to <span class="monospace">{{ store.sshAgentStatus.configPath }}</span>
            </summary>
            <p class="muted">It adds this section to the top, after saving a backup, so <span class="monospace">ssh</span> and <span class="monospace">git</span> use the SSH keys in your vaults:</p>
            <pre class="code-block monospace" data-testid="ssh-config-snippet">{{ store.sshAgentStatus.configSection }}</pre>
          </details>
          <p class="muted">
            {{
              store.sshAgentStatus.isTouchIDApproval
                ? "Each use of a key asks for Touch ID, unless you let the app skip asking for an hour."
                : "Each use of a key asks for your approval in Hisar, unless you let the app skip asking for an hour."
            }}
          </p>
        </div>
      </div>
      <p v-if="store.sshAgentErrorMessage" class="error settings-note" role="alert">{{ store.sshAgentErrorMessage }}</p>
    </div>

    <div v-if="store.sshAgentStatus" v-show="isRowVisible('compact-ssh-approvals') || isRowVisible('phone-approval')" class="settings-block">
      <p class="section-label">Approvals</p>
      <div class="settings-group">
        <label v-show="isRowVisible('compact-ssh-approvals')" class="setting">
          <div class="setting-text">
            <strong>Compact SSH approvals</strong>
            <p class="muted">Ask in a small card at the top right instead of a dialog; click the card for the details. Locked and forwarded requests still use the dialog.</p>
          </div>
          <input
            class="switch"
            type="checkbox"
            name="compactSSHApproval"
            :checked="store.sshAgentStatus.isCompactApproval"
            :disabled="store.isBusy"
            @change="setCompactSSHApproval(($event.target as HTMLInputElement).checked)"
          />
        </label>
        <label v-show="isRowVisible('phone-approval')" class="setting">
          <div class="setting-text">
            <strong>Approve requests on my iPhone</strong>
            <p class="muted">While Hisar is unlocked here, SSH and command-line requests also appear in Hisar on your iPhone when it is open and unlocked. Approving there never unlocks this Mac.</p>
          </div>
          <input
            class="switch"
            type="checkbox"
            name="phoneApproval"
            data-testid="phone-approval-switch"
            :checked="store.sshAgentStatus.isPhoneApproval"
            :disabled="store.isBusy"
            @change="setPhoneApproval(($event.target as HTMLInputElement).checked)"
          />
        </label>
      </div>
    </div>

    <div v-show="isRowVisible('commit-signing')" class="settings-block">
      <p class="section-label">Git</p>
      <div class="settings-group">
        <div class="setting" data-testid="commit-signing">
          <div class="setting-text">
            <strong>Remember commit signing</strong>
            <p class="muted">
              Git signs with an SSH key from your vaults through the agent (<span class="monospace">gpg.format ssh</span>). By default every signature
              asks. Allow remembering to get a “Don’t ask again” option in the signing dialog, for one app in one repository with one key, so a
              <span class="monospace">git rebase</span> asks once. Tags always ask; locking Hisar forgets it.
            </p>
          </div>
          <select
            v-if="store.sshAgentStatus"
            class="signing-remember"
            aria-label="Remember commit signing"
            data-testid="signing-remember"
            :value="store.sshAgentStatus.signingRememberSeconds"
            :disabled="store.isBusy"
            @change="setSigningRemember(Number(($event.target as HTMLSelectElement).value))"
          >
            <option v-for="option in allSigningRememberOption" :key="option.seconds" :value="option.seconds">{{ option.label }}</option>
          </select>
          <span v-else class="tag">Always asks</span>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.config-details {
  margin-bottom: 6px;
}

.config-details summary {
  cursor: pointer;
  font-size: 12px;
  color: var(--color-text-secondary);
}

.config-details > p {
  margin: 6px 0;
}

.signing-remember {
  flex: none;
  width: auto;
}

.state-dot.stopped {
  background: var(--color-danger);
}
</style>
