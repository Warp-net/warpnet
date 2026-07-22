<!-- Warpnet - Decentralized Social Network -->
<template>
  <!--
    Live mirror of cmd/node/member/auth/auth.go validatePassword. Keep
    the rules list and the regex literals in sync — if backend rules
    change, this file must too, otherwise the UI will report "OK" for
    a password the server then rejects.
  -->
  <ul class="mt-2 space-y-1 text-xs" aria-live="polite">
    <li v-for="rule in rules" :key="rule.label" class="flex items-center gap-2">
      <i
        :class="rule.ok
          ? 'fas fa-check-circle text-green-600'
          : 'far fa-circle text-dark'"
        aria-hidden="true"
      ></i>
      <span :class="rule.ok ? 'text-green-700' : 'text-dark'">
        {{ rule.label }}
      </span>
    </li>
  </ul>
</template>

<script>
import { passwordRules } from "@/lib/password";

export default {
  name: "PasswordRules",
  props: {
    password: { type: String, default: "" },
  },
  computed: {
    rules() {
      return passwordRules(this.password);
    },
  },
};
</script>
