<!-- Warpnet - Decentralized Social Network -->
<template>
  <div
    v-if="show"
    class="fixed inset-0 bg-black bg-opacity-50 z-50 flex items-center justify-center p-4"
    @click.self.stop="$emit('cancel')"
    @click.stop
  >
    <div class="bg-white rounded-lg w-full max-w-sm flex flex-col shadow-lg" @click.stop>
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
export default {
  name: "ConfirmDialog",
  props: {
    show: { type: Boolean, default: false },
    title: { type: String, default: "" },
    message: { type: String, required: true },
    confirmLabel: { type: String, default: "OK" },
    cancelLabel: { type: String, default: "Cancel" },
    destructive: { type: Boolean, default: false },
  },
  emits: ["confirm", "cancel"],
};
</script>
