<!-- Warpnet - Decentralized Social Network -->
<template>
  <div
    v-if="show"
    class="fixed inset-0 bg-black bg-opacity-50 z-50 flex items-center justify-center p-4"
    @click.self.stop="$emit('close')"
    @click.stop
  >
    <div class="bg-white rounded-lg w-full max-w-lg flex flex-col" @click.stop>
      <div class="px-5 py-3 border-b border-lighter flex items-center">
        <h2 class="font-bold text-lg">Retweet with comment</h2>
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
          maxlength="280"
          placeholder="Add a comment"
          class="w-full rounded border border-lighter bg-white p-2 focus:outline-none focus:ring-2 focus:ring-blue"
        ></textarea>
        <div class="text-right text-xs text-dark">{{ text.length }} / 280</div>

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
          >{{ saving ? 'Posting…' : 'Retweet' }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import {warpnetService} from "@/service/service";
import {toast} from "@/lib/toast";
import {dismissable} from "@/lib/modal.mixin";

export default {
  name: "QuoteOverlay",
  mixins: [dismissable("close")],
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
        // Quote semantics now ride on the regular retweet wire path —
        // a comment-bearing retweet carries the comment in `text` and
        // pins the source tweet via `quoted_tweet_id` / `quoted_user_id`.
        const result = await warpnetService.retweetTweet({
          tweetId: this.tweet.id,
          userId: this.tweet.user_id,
          username: this.tweet.username,
          text: this.tweet.text,
          comment: body,
        });
        this.$emit('posted', result);
        toast.success('Quote posted.');
        this.$emit('close');
      } catch (err) {
        console.error('Failed to retweet with comment:', err);
        toast.error(err?.message || "Couldn't post your quote. Please try again.");
      } finally {
        this.saving = false;
      }
    },
  },
};
</script>
