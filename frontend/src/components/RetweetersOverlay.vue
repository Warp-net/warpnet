<!-- Warpnet - Decentralized Social Network -->
<template>
  <div
    v-if="show"
    class="fixed inset-0 bg-black bg-opacity-50 z-50 flex items-center justify-center p-4"
    @click.self.stop="$emit('close')"
    @click.stop
  >
    <div class="bg-white rounded-lg w-full max-w-md max-h-[80vh] flex flex-col">
      <div class="px-5 py-3 border-b border-lighter flex items-center">
        <h2 class="font-bold text-lg">Retweeted by</h2>
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
        <div v-if="!loading && users.length === 0" class="p-5 text-center text-dark">
          No retweets yet
        </div>
        <div v-for="u in users" :key="u.id" class="px-5 py-3 border-b border-lighter flex items-center">
          <img :src="u.avatar || '/default_profile.png'" class="h-10 w-10 rounded-full object-cover" alt="" />
          <button
            @click="goProfile(u.id)"
            class="ml-3 text-left flat-btn"
          >
            <p class="font-bold">{{ u.username || u.id }}</p>
            <p class="text-dark text-sm">@{{ u.id }}</p>
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
  name: "RetweetersOverlay",
  components: {
    Loader: defineAsyncComponent(() => import('@/components/Loader.vue')),
  },
  props: {
    show: { type: Boolean, default: false },
    tweetId: { type: String, required: true },
    ownerUserId: { type: String, required: true },
  },
  emits: ['close'],
  data() {
    return { loading: true, users: [] };
  },
  watch: {
    show: {
      immediate: true,
      handler(v) {
        if (v) this.load();
      },
    },
  },
  methods: {
    goProfile(id) {
      this.$emit('close');
      this.$router.push({ name: 'Profile', params: { id } });
    },
    async load() {
      this.loading = true;
      try {
        const resp = await warpnetService.getTweetRetweeters(this.tweetId, this.ownerUserId);
        const list = resp?.users || [];
        this.users = await Promise.all(list.map(async (u) => {
          try {
            if (u.avatar_key && !u.avatar) {
              u.avatar = await warpnetService.getImage({userId: u.id, key: u.avatar_key});
            }
          } catch (e) {}
          return u;
        }));
      } catch (err) {
        console.error('Failed to load retweeters:', err);
        this.users = [];
      } finally {
        this.loading = false;
      }
    },
  },
};
</script>
