<!-- Warpnet - Decentralized Social Network -->
<template>
  <div
    v-if="show"
    class="fixed inset-0 bg-black bg-opacity-50 z-50 flex items-center justify-center p-4"
    @click.self="$emit('close')"
  >
    <div class="bg-white rounded-lg w-full max-w-lg flex flex-col">
      <div class="px-5 py-3 border-b border-lighter flex items-center">
        <h2 class="font-bold text-lg">Quote tweet</h2>
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
          placeholder="Add a comment"
          class="w-full rounded border border-lighter bg-white p-2 focus:outline-none focus:ring-2 focus:ring-blue"
        ></textarea>
        <div class="text-right text-xs text-dark">{{ text.length }} / 2000</div>

        <div class="mt-3 border border-lighter rounded p-3 bg-lightest text-sm">
          <p class="font-bold">{{ tweet.username || 'Anonymous' }}
            <span class="text-dark font-normal ml-1">@{{ tweet.user_id }}</span>
          </p>
          <p class="mt-1 line-clamp-3">{{ tweet.text }}</p>
        </div>

        <div class="flex justify-end gap-2 mt-3">
          <button
            @click="$emit('close')"
            class="px-3 py-1 rounded-full border border-lighter hover:bg-lighter"
          >Cancel</button>
          <button
            @click="submit"
            :disabled="saving || !text.trim()"
            class="text-white bg-blue rounded-full font-semibold px-4 py-1 hover:bg-darkblue disabled:opacity-50"
          >{{ saving ? 'Posting…' : 'Quote' }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import {warpnetService} from "@/service/service";

export default {
  name: "QuoteOverlay",
  props: {
    show: { type: Boolean, default: false },
    tweet: { type: Object, required: true },
  },
  emits: ['close', 'posted'],
  data() {
    return { text: '', saving: false };
  },
  watch: {
    show(v) { if (v) this.text = ''; },
  },
  methods: {
    async submit() {
      const body = this.text.trim();
      if (!body) return;
      this.saving = true;
      try {
        const result = await warpnetService.quoteTweet(this.tweet.id, this.tweet.user_id, body);
        this.$emit('posted', result);
        this.$emit('close');
      } catch (err) {
        console.error('Failed to quote tweet:', err);
      } finally {
        this.saving = false;
      }
    },
  },
};
</script>
