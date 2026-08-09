<!-- Warpnet - Decentralized Social Network -->
<template>
  <div v-if="items.length" class="toast-container toast-stack">
    <div
      v-for="t in items"
      :key="t.id"
      :class="['toast', `toast-${t.type}`]"
      role="alert"
    >
      <i :class="iconFor(t.type)" aria-hidden="true"></i>
      <span class="flex-1">{{ t.message }}</span>
      <button
        v-if="t.action"
        type="button"
        class="toast-btn font-semibold underline flat-btn"
        @click="runAction(t)"
      >{{ t.action.label }}</button>
      <button
        type="button"
        class="toast-btn flat-btn"
        aria-label="Dismiss"
        @click="dismiss(t.id)"
      ><i class="fas fa-times" aria-hidden="true"></i></button>
    </div>
  </div>
</template>

<script>
import { toastState, dismissToast } from "@/lib/toast";

export default {
  name: "ToastHost",
  computed: {
    items() {
      return toastState.items;
    },
  },
  methods: {
    dismiss(id) {
      dismissToast(id);
    },
    runAction(t) {
      try {
        if (t.action && typeof t.action.onClick === "function") t.action.onClick();
      } finally {
        dismissToast(t.id);
      }
    },
    iconFor(type) {
      if (type === "error") return "fas fa-exclamation-circle";
      if (type === "success") return "fas fa-check-circle";
      return "fas fa-info-circle";
    },
  },
};
</script>

<style scoped>
/* Stack multiple toasts vertically; base .toast/.toast-* styling lives in
   tailwind.css so it stays consistent with the pre-existing toast markup. */
.toast-stack {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
}
.toast-btn {
  background: transparent;
  border: none;
  cursor: pointer;
  color: inherit;
  padding: 0 0.25rem;
  flex-shrink: 0;
}
</style>
