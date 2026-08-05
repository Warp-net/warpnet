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
        v-if="!loading && reactions.length === 0"
        class="flex flex-col items-center justify-center pt-10 px-5"
      >
        <p class="font-bold text-lg">Nothing reacted yet</p>
        <p class="text-sm text-dark text-center">
          Tweets you like will show up here so you can always find your way back to them.
        </p>
      </div>

      <template v-for="l in reactions" :key="l.tweet_id || l.tweet?.id">
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
      reactions: [],
      done: false,
      ownerProfile: {},
    };
  },
  methods: {
    goBack() {
      if (window.history.length > 1) this.$router.back();
      else this.$router.push({ name: "Home" });
    },
    async hydrateReactions(items) {
      await Promise.all(items.map(async (l) => {
        try {
          const tweet = await warpnetService.getTweet({
            userId: l.owner_user_id || this.ownerProfile.user_id,
            tweetId: l.tweet_id,
          });
          if (tweet && tweet.id) l.tweet = tweet;
        } catch (e) {
          console.warn('reaction hydrate failed:', l, e);
        }
      }));
    },
    async loadMore() {
      if (this.done || this.loading) return;
      const resp = await warpnetService.getReactions(false);
      const items = resp?.items || [];
      if (items.length === 0) { this.done = true; return; }
      this.reactions = this.reactions.concat(items);
      this.hydrateReactions(this.reactions.slice(-items.length));
    },
  },
  async created() {
    console.log("loading component:", this.$options.name);
    this.ownerProfile = warpnetService.getOwnerProfile();
    try {
      const resp = await warpnetService.getReactions(true);
      this.reactions = resp?.items || [];
      if (this.reactions.length === 0 && (resp?.cursor === 'end')) this.done = true;
      await Promise.race([
        this.hydrateReactions(this.reactions),
        new Promise((resolve) => setTimeout(resolve, 1000)),
      ]);
    } catch (err) {
      console.error('Failed to load reactions:', err);
    } finally {
      this.loading = false;
    }
  },
};
</script>
