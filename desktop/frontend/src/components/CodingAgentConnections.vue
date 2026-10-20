<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { api, getErrorMessage, type CodingAgent } from "../api";
import { store } from "../store";
import AppIcon from "./AppIcon.vue";

interface AgentCopy {
  notInstalled: string;
  // What connecting gives, around the command it goes through: [before, command, after].
  purpose: [string, string, string];
}

const allCopyByAgentID: Record<string, AgentCopy> = {
  omp: {
    notInstalled: "omp is not installed. Install omp, or Tern, which comes with it.",
    purpose: ["Connect to let omp sessions run commands with your secrets through ", "hisar run", " and show Hisar's state in their status line."],
  },
  "claude-code": {
    notInstalled: "Claude Code is not installed.",
    purpose: ["Connect to let Claude Code run approved commands with your secrets through ", "hisar mcp", ". Commands receive the real values; output masking is not containment."],
  },
  codex: {
    notInstalled: "Codex is not installed.",
    purpose: ["Connect to let Codex run approved commands with your secrets through ", "hisar mcp", ". Commands receive the real values; output masking is not containment."],
  },
};

function getCopy(agent: CodingAgent): AgentCopy {
  return (
    allCopyByAgentID[agent.id] ?? {
      notInstalled: `${agent.name} is not installed.`,
      purpose: [`Connect to let ${agent.name} run approved commands with your secrets through `, "hisar mcp", ". Commands receive the real values; output masking is not containment."],
    }
  );
}

const allAgent = ref<CodingAgent[] | null>(null);
const changingAgentID = ref("");
const failure = ref<{ agentID: string; message: string } | null>(null);

const isVaultNeeded = computed(
  () => store.allAgentVaultID.length === 0 && (allAgent.value ?? []).some((agent) => agent.state === "available" || agent.state === "connected"),
);

async function loadAgents(): Promise<void> {
  try {
    allAgent.value = await api.listAllCodingAgent();
  } catch (error) {
    allAgent.value = [];
    failure.value = { agentID: "", message: getErrorMessage(error) };
  }
}

async function applyChange(agent: CodingAgent, change: (id: string) => Promise<CodingAgent>): Promise<void> {
  changingAgentID.value = agent.id;
  failure.value = null;
  try {
    const changedAgent = await change(agent.id);
    allAgent.value = (allAgent.value ?? []).map((existing) => (existing.id === changedAgent.id ? changedAgent : existing));
  } catch (error) {
    failure.value = { agentID: agent.id, message: getErrorMessage(error) };
  } finally {
    changingAgentID.value = "";
  }
}

const cursorSettings = `{
  "mcpServers": {
    "hisar": { "command": "hisar", "args": ["mcp", "--no-reveal"] }
  }
}`;

// The vault switches sit below the agents in the same section.
function showVaultSwitches(): void {
  document.querySelector<HTMLInputElement>('input[name^="agentVault-"]')?.focus();
}

onMounted(loadAgents);
</script>

<template>
  <div class="settings-group agent-connections" data-testid="agent-connections">
    <p v-if="allAgent === null" class="muted empty-row">Looking for coding agents…</p>
    <div v-for="agent in allAgent ?? []" :key="agent.id" class="setting" :data-testid="`agent-card-${agent.id}`">
      <span class="settings-row-icon"><AppIcon name="terminal" :size="14" /></span>
      <div class="setting-text">
        <strong>{{ agent.name }}</strong>
        <p v-if="agent.state === 'not-installed'" class="muted" :data-testid="`agent-state-${agent.id}`">
          <span class="state-dot"></span>{{ getCopy(agent).notInstalled }}
        </p>
        <p v-else-if="agent.state === 'available'" class="muted" :data-testid="`agent-state-${agent.id}`">
          {{ getCopy(agent).purpose[0] }}<span class="monospace">{{ getCopy(agent).purpose[1] }}</span>{{ getCopy(agent).purpose[2] }}
        </p>
        <template v-else-if="agent.state === 'connected'">
          <p class="muted" :data-testid="`agent-state-${agent.id}`"><span class="state-dot running"></span>Connected</p>
          <p class="muted" :data-testid="`agent-restart-hint-${agent.id}`">New {{ agent.name }} sessions load Hisar. Restart the ones already running.</p>
        </template>
        <p v-else class="warning-text" :data-testid="`agent-state-${agent.id}`"><span class="state-dot warning"></span>{{ agent.detail }}</p>
        <p v-if="failure && failure.agentID === agent.id" class="error" role="alert" :data-testid="`agent-error-${agent.id}`">{{ failure.message }}</p>
      </div>
      <button
        v-if="agent.state === 'available'"
        class="primary"
        :data-testid="`connect-agent-${agent.id}`"
        :disabled="changingAgentID !== ''"
        @click="applyChange(agent, api.connectCodingAgent)"
      >
        Connect
      </button>
      <button
        v-else-if="agent.state === 'connected'"
        :data-testid="`disconnect-agent-${agent.id}`"
        :disabled="changingAgentID !== ''"
        @click="applyChange(agent, api.disconnectCodingAgent)"
      >
        Disconnect
      </button>
    </div>
    <div v-if="isVaultNeeded" class="settings-group-detail">
      <p class="warning-text" data-testid="agents-no-vaults">
        Coding agents see no secrets until a vault is available to agents.
        <button class="link" data-testid="show-agent-vaults" @click="showVaultSwitches">Choose vaults</button>
      </p>
    </div>
    <div class="setting">
      <span class="settings-row-icon"><AppIcon name="code" :size="14" /></span>
      <div class="setting-text">
        <strong>Other MCP clients</strong>
        <p class="muted">Add <span class="monospace">hisar mcp --no-reveal</span> to Cursor or another MCP client. In Cursor, put this in ~/.cursor/mcp.json:</p>
      </div>
    </div>
    <div class="settings-group-detail">
      <pre class="code-block monospace" data-testid="coding-agents-snippet">{{ cursorSettings }}</pre>
    </div>
  </div>
  <p v-if="failure && failure.agentID === ''" class="error settings-note" role="alert" data-testid="agent-connections-error">{{ failure.message }}</p>
</template>

<style scoped>
.agent-connections {
  margin-bottom: 10px;
}

.empty-row {
  margin: 0;
  padding: 10px 14px;
  font-size: 12px;
}
</style>
