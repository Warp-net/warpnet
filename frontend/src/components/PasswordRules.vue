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
const MIN = 8;
const MAX = 32;

export default {
  name: "PasswordRules",
  props: {
    password: { type: String, default: "" },
  },
  computed: {
    rules() {
      const p = this.password || "";
      return [
        {
          label: `${MIN}–${MAX} characters`,
          ok: p.length >= MIN && p.length <= MAX,
        },
        { label: "one uppercase letter (A–Z)", ok: /[A-Z]/.test(p) },
        { label: "one lowercase letter (a–z)", ok: /[a-z]/.test(p) },
        { label: "one digit (0–9)", ok: /[0-9]/.test(p) },
        { label: "one special character", ok: /[\W_]/.test(p) },
      ];
    },
  },
};
</script>
