<!-- Warpnet - Decentralized Social Network -->
<template>
  <div id="app" class="flex container h-screen w-full">
    <SideNav />
    <div
      class="w-full h-full overflow-y-scroll no-scrollbar"
      v-scroll:bottom="loadMore"
    >
      <div class="px-5 py-3 border-b border-lighter flex items-center">
        <button
          @click="goBack()"
          class="rounded-full md:pr-2 focus:outline-none hover:bg-lightblue"
          aria-label="Back"
        >
          <i class="fas fa-arrow-left text-blue"></i>
        </button>
        <h1 class="text-xl font-bold ml-4">Likes</h1>
      </div>

      <Loader :loading="loading" />

      <div
        v-if="!loading && likes.length === 0"
        class="flex flex-col items-center justify-center pt-10 px-5"
      >
        <p class="font-bold text-lg">Nothing liked yet</p>
        <p class="text-sm text-dark text-center">
          Tweets you like will show up here so you can always find your way back to them.
        </p>
      </div>

      <template v-for="l in likes" :key="l.tweet_id || l.tweet?.id">
        <TweetBlock v-if="l.tweet && l.tweet.id" :tweet="l.tweet" />
      </template>
    </div>
    <DefaultRightBar :profile="ownerProfile" />
  </div>
</template>

<script>
import {defineAsyncComponent} from "vue";
import {warpnetService} from "@/service/service";

export default {
  name: "Likes",
  components: {
    SideNav: defineAsyncComponent(() => import('@/components/SideNav.vue')),
    DefaultRightBar: defineAsyncComponent(() => import('@/components/DefaultRightBar.vue')),
    Loader: defineAsyncComponent(() => import('@/components/Loader.vue')),
    TweetBlock: defineAsyncComponent(() => import('@/components/TweetBlock.vue')),
  },
  data() {
    return {
      loading: true,
      likes: [],
      done: false,
      ownerProfile: {},
    };
  },
  methods: {
    goBack() {
      if (window.history.length > 1) this.$router.back();
      else this.$router.push({ name: "Home" });
    },
    async loadMore() {
      if (this.done || this.loading) return;
      const resp = await warpnetService.getLikes(false);
      const items = resp?.items || [];
      if (items.length === 0) { this.done = true; return; }
      this.likes = this.likes.concat(items);
    },
  },
  async created() {
    console.log("loading component:", this.$options.name);
    this.ownerProfile = warpnetService.getOwnerProfile();
    try {
      const resp = await warpnetService.getLikes(true);
      this.likes = resp?.items || [];
      if (this.likes.length === 0 && (resp?.cursor === 'end')) this.done = true;
    } catch (err) {
      console.error('Failed to load likes:', err);
    } finally {
      this.loading = false;
    }
  },
};
</script>
