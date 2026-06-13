<!-- Warpnet - Decentralized Social Network -->
<template>
  <div
    v-if="show"
    class="fixed inset-0 bg-black bg-opacity-50 z-50 flex items-center justify-center p-4"
    role="dialog"
    aria-modal="true"
    :aria-labelledby="titleId"
    :aria-describedby="descId"
    @click.self.stop="cancel"
    @click.stop
  >
    <div class="bg-white rounded-lg w-full max-w-sm flex flex-col shadow-lg" @click.stop>
      <div class="px-5 py-4">
        <h2 :id="titleId" class="font-bold text-lg mb-2">{{ title }}</h2>
        <p :id="descId" class="text-sm text-dark mb-3">
          Reports are sent to moderators on the network. The reported user is not notified.
        </p>
        <fieldset>
          <legend class="block text-sm font-medium mb-1">
            What's wrong? Select all that apply.
          </legend>
          <label
            v-for="category in categories"
            :key="category"
            class="flex items-center gap-2 py-1 text-sm cursor-pointer"
          >
            <input
              type="checkbox"
              :value="category"
              v-model="selected"
              class="text-red-600 focus:ring-red-500"
            />
            <span>{{ category }}</span>
          </label>
        </fieldset>
      </div>
      <div class="flex justify-end gap-2 px-5 py-3 border-t border-lighter">
        <button
          @click.stop="cancel"
          class="px-4 py-1 rounded-full border border-lighter hover:bg-lighter"
        >Cancel</button>
        <button
          :disabled="!canSubmit"
          @click.stop="submit"
          class="px-4 py-1 rounded-full font-semibold text-white bg-red-600 hover:bg-red-700 disabled:opacity-50"
        >{{ submitting ? "Sending..." : "Submit report" }}</button>
      </div>
    </div>
  </div>
</template>

<script>
// Violation categories mirror the active Llama Guard hazard classes the
// moderation engine reports (see the `moderation` repo, prompt.go —
// llamaGuardCategories). Keeping the labels identical means a user's
// report speaks the same language as the automated verdict.
const REPORT_CATEGORIES = [
  "Violent Crimes",
  "Non-Violent Crimes",
  "Sex Crimes",
  "Child Exploitation",
  "Privacy",
  "Indiscriminate Weapons",
  "Hate",
  "Self-Harm",
  "Code Interpreter Abuse",
];

export default {
  name: "ReportDialog",
  props: {
    show: { type: Boolean, default: false },
    title: { type: String, default: "Report" },
  },
  emits: ["submit", "cancel"],
  data() {
    // Uniqued per instance so multiple report dialogs on the same
    // page (e.g. one in a tweet list, another on a profile) don't
    // share ARIA ids.
    const uid = Math.random().toString(36).slice(2, 8);
    return {
      selected: [],
      submitting: false,
      categories: REPORT_CATEGORIES,
      titleId: `report-dialog-title-${uid}`,
      descId: `report-dialog-desc-${uid}`,
    };
  },
  computed: {
    // The wire reason is the comma-joined list of selected categories,
    // so moderators see the standardized labels.
    composedReason() {
      return this.selected.join(", ");
    },
    canSubmit() {
      return !this.submitting && this.selected.length > 0;
    },
  },
  watch: {
    show(val) {
      if (val) {
        this.selected = [];
        this.submitting = false;
      }
    },
  },
  methods: {
    cancel() {
      if (this.submitting) return;
      this.$emit("cancel");
    },
    submit() {
      if (!this.canSubmit) return;
      this.submitting = true;
      this.$emit("submit", this.composedReason);
    },
  },
};
</script>
