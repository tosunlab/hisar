<script setup lang="ts">
import AccountDialog from "./AccountDialog.vue";
const emit = defineEmits<{ close: [] }>();
</script>

<template>
  <AccountDialog title="Can't find your Secret Key?" icon="key" @close="emit('close')">
    <p>It's on your Emergency Kit, the PDF or printout you saved when you created your account. If you saved the Secret Key separately, you can use that too.</p>
    <p>Still signed in on another device? Unlock Hisar there with your master password and save a new Emergency Kit: on a Mac in Settings, Security; on an iPhone in Settings, Account, Secret Key and Emergency Kit; on Android in Settings, Account, Secret Key and Emergency Kit. Don't sign out of that device first.</p>
    <p>If this device offers Finish account setup for an unfinished operation, try that with the master password you chose for it.</p>
    <p>Without your Secret Key, a usable signed-in device or a protected unfinished setup you can resume, neither Hisar nor the server's admin can recover your account.</p>
    <h3>Forgot your master password?</h3>
    <p>Hisar can't reset it, and neither can the server's admin. The Emergency Kit doesn't replace it. If you can still open Hisar on a device, don't sign out; preserve the items you can access. If you can no longer unlock any device and cannot find your master password, items only your account can open cannot be recovered. Shared vaults remain accessible to their other members.</p>
    <div class="row actions"><button class="primary" @click="emit('close')">Done</button></div>
  </AccountDialog>
</template>
