<!-- Warpnet - Decentralized Social Network -->
<template>
  <ConfirmDialog
    :show="show"
    title="Turn on the wallet?"
    :message="message"
    :confirm-label="busy ? 'Turning on…' : 'Turn on'"
    :confirm-disabled="busy"
    @confirm="turnOn"
    @cancel="$emit('cancel')"
  />
</template>

<script>
import {defineAsyncComponent} from "vue";
import {warpnetService} from "@/service/service";
import {toast} from "@/lib/toast";

export default {
  name: "WalletSwitchDialog",
  components: {
    ConfirmDialog: defineAsyncComponent(() => import('./ConfirmDialog.vue')),
  },
  props: {
    show: { type: Boolean, default: false },
    reason: { type: String, required: true },
  },
  emits: ["on", "cancel"],
  data() {
    return { busy: false };
  },
  computed: {
    message() {
      return `${this.reason}\n\nThe wallet comes from your account and lives on your node. Turning it on starts the payment engine; you can turn it off again in Settings.`;
    },
  },
  methods: {
    async turnOn() {
      if (this.busy) return;
      this.busy = true;
      try {
        await warpnetService.turnWalletOn();
        this.$emit("on");
      } catch (err) {
        console.error("Failed to turn the wallet on:", err);
        toast.error(err?.message || "Failed to turn the wallet on.");
      } finally {
        this.busy = false;
      }
    },
  },
};
</script>
