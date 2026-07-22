<!-- Warpnet - Decentralized Social Network -->
<template>
  <div
    v-if="show"
    class="fixed inset-0 bg-black bg-opacity-50 z-50 flex items-center justify-center p-4"
    @click.self.stop="$emit('cancel')"
    @click.stop
  >
    <div class="bg-white dark:bg-darktheme-card mastodon:bg-mastodon-card rounded-lg w-full max-w-sm flex flex-col shadow-lg" @click.stop>
      <div class="px-5 py-4">
        <h2 v-if="title" class="font-bold text-lg mb-2">{{ title }}</h2>
        <p class="text-sm text-dark whitespace-pre-line">{{ message }}</p>
      </div>
      <div class="flex justify-end gap-2 px-5 py-3 border-t border-lighter">
        <button
          @click.stop="$emit('cancel')"
          class="px-4 py-1 rounded-full border border-lighter hover:bg-lighter"
        >{{ cancelLabel }}</button>
        <button
          @click.stop="$emit('confirm')"
          class="px-4 py-1 rounded-full font-semibold text-white"
          :class="destructive
            ? 'bg-red-600 hover:bg-red-700'
            : 'bg-blue hover:bg-darkblue'"
        >{{ confirmLabel }}</button>
      </div>
    </div>
  </div>
</template>

<script>
import {dismissable} from "@/lib/modal.mixin";

export default {
  name: "ConfirmDialog",
  mixins: [dismissable({ handler: "onEscape" })],
  props: {
    show: { type: Boolean, default: false },
    title: { type: String, default: "" },
    message: { type: String, required: true },
    confirmLabel: { type: String, default: "OK" },
    cancelLabel: { type: String, default: "Cancel" },
    destructive: { type: Boolean, default: false },
  },
  emits: ["confirm", "cancel"],
  methods: {
    // Escape cancels — but only while the dialog is actually shown, so a
    // stray Escape elsewhere doesn't fire a phantom cancel.
    onEscape() {
      if (this.show) this.$emit("cancel");
    },
  },
};
</script>
