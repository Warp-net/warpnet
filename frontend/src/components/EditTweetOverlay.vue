<!-- Warpnet - Decentralized Social Network -->
<template>
  <div
    v-if="show"
    class="fixed inset-0 bg-black bg-opacity-50 z-50 flex items-center justify-center p-4"
    @click.self.stop="$emit('close')"
    @click.stop
  >
    <div class="bg-white rounded-lg w-full max-w-lg flex flex-col">
      <div class="px-5 py-3 border-b border-lighter flex items-center">
        <h2 class="font-bold text-lg">Edit tweet</h2>
        <button
          @click="$emit('close')"
          class="ml-auto text-dark hover:text-black"
          aria-label="Close"
        >
          <i class="fas fa-times"></i>
        </button>
      </div>
      <div class="p-5">
        <textarea
          v-model="text"
          rows="4"
          maxlength="2000"
          placeholder="Edit your tweet"
          class="w-full rounded border border-lighter bg-white p-2 focus:outline-none focus:ring-2 focus:ring-blue"
        ></textarea>
        <div class="text-right text-xs text-dark">{{ text.length }} / 2000</div>
        <div class="flex justify-end gap-2 mt-3">
          <button
            @click="$emit('close')"
            class="px-3 py-1 rounded-full border border-lighter hover:bg-lighter"
          >Cancel</button>
          <button
            @click="save"
            :disabled="saving || !text.trim()"
            class="text-white bg-blue rounded-full font-semibold px-4 py-1 hover:bg-darkblue disabled:opacity-50"
          >{{ saving ? 'Saving…' : 'Save' }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import {warpnetService} from "@/service/service";

export default {
  name: "EditTweetOverlay",
  props: {
    show: { type: Boolean, default: false },
    tweet: { type: Object, required: true },
  },
  emits: ['close', 'saved'],
  data() {
    return { text: '', saving: false };
  },
  watch: {
    show: {
      immediate: true,
      handler(v) {
        if (v) this.text = this.tweet?.text || '';
      },
    },
  },
  methods: {
    async save() {
      const newText = this.text.trim();
      if (!newText) return;
      this.saving = true;
      try {
        const updated = await warpnetService.editTweet(this.tweet.id, newText);
        this.$emit('saved', updated || { ...this.tweet, text: newText });
        this.$emit('close');
      } catch (err) {
        console.error('Failed to edit tweet:', err);
      } finally {
        this.saving = false;
      }
    },
  },
};
</script>
