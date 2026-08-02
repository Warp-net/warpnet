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
          ref="composer"
          v-model="text"
          rows="4"
          placeholder="Edit your tweet"
          class="w-full rounded border border-lighter bg-white p-2 focus:outline-none focus:ring-2 focus:ring-blue"
        ></textarea>
        <div class="flex items-center justify-between">
          <div class="relative" data-emoji-anchor>
            <button
              type="button"
              @click="showEmojiPicker = !showEmojiPicker"
              class="text-lg text-blue rounded-full w-9 h-9 flex items-center justify-center hover:bg-lightblue"
              aria-label="Add emoji"
              title="Add emoji"
              :aria-expanded="showEmojiPicker"
            >
              <i class="far fa-smile" aria-hidden="true"></i>
            </button>
            <EmojiPicker
              v-if="showEmojiPicker"
              @select="insertEmoji"
              @close="showEmojiPicker = false"
            />
          </div>
          <span class="text-xs text-dark">{{ textLength }} / 280</span>
        </div>
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
import {toast} from "@/lib/toast";
import {defineAsyncComponent} from "vue";
import {dismissable} from "@/lib/modal.mixin";
import {clampRunes, focusCaret, insertEmoji, runeLength} from "@/lib/emoji";

const tweetCharLimit = 280;

export default {
  name: "EditTweetOverlay",
  components: {
    EmojiPicker: defineAsyncComponent(() => import('@/components/EmojiPicker.vue')),
  },
  // Escape closes the emoji picker first when it is open, so a stray Escape
  // never throws away a draft that is still being written.
  mixins: [dismissable({handler: "onEscape"})],
  props: {
    show: { type: Boolean, default: false },
    tweet: { type: Object, required: true },
  },
  emits: ['close', 'saved'],
  data() {
    return { text: '', saving: false, showEmojiPicker: false };
  },
  computed: {
    textLength() {
      return runeLength(this.text);
    },
  },
  watch: {
    // Runes, not UTF-16 units: matches the 280 the node enforces.
    text(value) {
      const clamped = clampRunes(value, tweetCharLimit);
      if (clamped !== value) this.text = clamped;
    },
    show: {
      immediate: true,
      handler(v) {
        if (v) this.text = this.tweet?.text || '';
      },
    },
  },
  methods: {
    insertEmoji(emoji) {
      const field = this.$refs.composer;
      const next = insertEmoji({
        text: this.text,
        emoji,
        field,
        limit: tweetCharLimit,
      });
      if (!next) return;
      this.text = next.text;
      this.$nextTick(() => focusCaret(field, next.caret));
    },
    onEscape() {
      if (this.showEmojiPicker) return;
      this.$emit("close");
    },
    async save() {
      const newText = this.text.trim();
      if (!newText) return;
      this.saving = true;
      try {
        const updated = await warpnetService.editTweet(this.tweet.id, newText);
        this.$emit('saved', updated || { ...this.tweet, text: newText });
        toast.success('Tweet updated.');
        this.$emit('close');
      } catch (err) {
        console.error('Failed to edit tweet:', err);
        toast.error(err?.message || "Couldn't save your changes. Please try again.");
      } finally {
        this.saving = false;
      }
    },
  },
};
</script>
