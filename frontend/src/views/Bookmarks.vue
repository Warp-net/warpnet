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
          @click="$router.push({ name: 'Home' })"
          class="rounded-full md:pr-2 focus:outline-none hover:bg-lightblue"
          aria-label="Back"
        >
          <i class="fas fa-arrow-left text-blue"></i>
        </button>
        <h1 class="text-xl font-bold ml-4">Bookmarks</h1>
      </div>

      <Loader :loading="loading" />

      <div
        v-if="!loading && bookmarks.length === 0"
        class="flex flex-col items-center justify-center pt-10 px-5"
      >
        <p class="font-bold text-lg">Save tweets for later</p>
        <p class="text-sm text-dark text-center">
          Bookmark tweets to easily find them again in the future. Your bookmarks are private — only you can see them.
        </p>
      </div>

      <template v-for="b in bookmarks" :key="b.tweet_id || b.tweet?.id">
        <TweetBlock v-if="b.tweet && b.tweet.id" :tweet="b.tweet" />
      </template>
    </div>
    <DefaultRightBar :profile="ownerProfile" />
  </div>
</template>

<script>
import {defineAsyncComponent} from "vue";
import {warpnetService} from "@/service/service";

export default {
  name: "Bookmarks",
  components: {
    SideNav: defineAsyncComponent(() => import('@/components/SideNav.vue')),
    DefaultRightBar: defineAsyncComponent(() => import('@/components/DefaultRightBar.vue')),
    Loader: defineAsyncComponent(() => import('@/components/Loader.vue')),
    TweetBlock: defineAsyncComponent(() => import('@/components/TweetBlock.vue')),
  },
  data() {
    return {
      loading: true,
      bookmarks: [],
      done: false,
      ownerProfile: {},
    };
  },
  methods: {
    async loadMore() {
      if (this.done || this.loading) return;
      const resp = await warpnetService.getBookmarks(false);
      const items = resp?.items || [];
      if (items.length === 0) { this.done = true; return; }
      this.bookmarks = this.bookmarks.concat(items);
    },
  },
  async created() {
    console.log("loading component:", this.$options.name);
    this.ownerProfile = warpnetService.getOwnerProfile();
    try {
      const resp = await warpnetService.getBookmarks(true);
      this.bookmarks = resp?.items || [];
      if (this.bookmarks.length === 0 && (resp?.cursor === 'end')) this.done = true;
    } catch (err) {
      console.error('Failed to load bookmarks:', err);
    } finally {
      this.loading = false;
    }
  },
};
</script>
