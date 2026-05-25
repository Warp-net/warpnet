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
        <fieldset class="space-y-2">
          <legend class="sr-only">Reason</legend>
          <label v-for="opt in options" :key="opt.value" class="flex items-center gap-2 cursor-pointer">
            <input
              type="radio"
              :name="radioGroupName"
              v-model="selected"
              :value="opt.value"
              class="cursor-pointer"
            />
            <span class="text-sm">{{ opt.label }}</span>
          </label>
        </fieldset>
      </div>
      <div class="flex justify-end gap-2 px-5 py-3 border-t border-lighter">
        <button
          @click.stop="cancel"
          class="px-4 py-1 rounded-full border border-lighter hover:bg-lighter"
        >Cancel</button>
        <button
          :disabled="!selected || submitting"
          @click.stop="submit"
          class="px-4 py-1 rounded-full font-semibold text-white bg-red-600 hover:bg-red-700 disabled:opacity-50"
        >{{ submitting ? "Sending..." : "Submit report" }}</button>
      </div>
    </div>
  </div>
</template>

<script>
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
    // share ARIA ids or a radio-group name.
    const uid = Math.random().toString(36).slice(2, 8);
    return {
      selected: "",
      submitting: false,
      titleId: `report-dialog-title-${uid}`,
      descId: `report-dialog-desc-${uid}`,
      radioGroupName: `report-reason-${uid}`,
      options: [
        { value: "spam", label: "Spam" },
        { value: "abuse", label: "Abuse or harassment" },
        { value: "illegal", label: "Illegal content" },
        { value: "nsfw", label: "NSFW / explicit content" },
      ],
    };
  },
  watch: {
    show(val) {
      if (val) {
        this.selected = "";
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
      if (!this.selected) return;
      this.submitting = true;
      this.$emit("submit", this.selected);
    },
  },
};
</script>
