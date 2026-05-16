<!-- Warpnet - Decentralized Social Network -->
<template>
  <div
    v-if="show"
    class="fixed inset-0 bg-black bg-opacity-50 z-50 flex items-center justify-center p-4"
    @click.self="$emit('close')"
  >
    <div class="bg-white rounded-lg w-full max-w-md max-h-[80vh] flex flex-col">
      <div class="px-5 py-3 border-b border-lighter flex items-center">
        <h2 class="font-bold text-lg">Quotes</h2>
        <button
          @click="$emit('close')"
          class="ml-auto text-dark hover:text-black"
          aria-label="Close"
        >
          <i class="fas fa-times"></i>
        </button>
      </div>
      <div class="overflow-y-auto flex-1">
        <Loader :loading="loading" />
        <div v-if="!loading && quotes.length === 0" class="p-5 text-center text-dark">
          No quotes yet
        </div>
        <div v-for="q in quotes" :key="q.id" class="px-5 py-3 border-b border-lighter">
          <button
            class="text-left w-full flat-btn hover:bg-lightest"
            @click="openQuote(q)"
          >
            <p class="font-bold">{{ q.username || q.user_id }}
              <span class="text-dark font-normal text-sm ml-1">@{{ q.user_id }}</span>
            </p>
            <p class="mt-1 text-sm line-clamp-3">{{ q.text }}</p>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import {defineAsyncComponent} from "vue";
import {warpnetService} from "@/service/service";

export default {
  name: "QuotingOverlay",
  components: {
    Loader: defineAsyncComponent(() => import('@/components/Loader.vue')),
  },
  props: {
    show: { type: Boolean, default: false },
    tweetId: { type: String, required: true },
    ownerUserId: { type: String, required: true },
  },
  emits: ['close'],
  data() { return { loading: true, quotes: [] }; },
  watch: {
    show: {
      immediate: true,
      handler(v) { if (v) this.load(); },
    },
  },
  methods: {
    openQuote(q) {
      this.$emit('close');
      this.$router.push({
        name: 'Tweet',
        params: { id: q.id },
        query: { u: q.user_id || '' },
      });
    },
    async load() {
      this.loading = true;
      try {
        const resp = await warpnetService.getQuoting(this.tweetId, this.ownerUserId);
        this.quotes = resp?.tweets || resp?.quotes || [];
      } catch (err) {
        console.error('Failed to load quotes:', err);
        this.quotes = [];
      } finally {
        this.loading = false;
      }
    },
  },
};
</script>
