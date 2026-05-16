<!-- Warpnet - Decentralized Social Network -->
<template>
  <div id="app" class="flex container h-screen w-full">
    <SideNav />
    <div class="w-full h-full overflow-y-scroll no-scrollbar">
      <div class="px-5 py-3 border-b border-lighter flex items-center">
        <button
          @click="$router.back()"
          class="rounded-full md:pr-2 focus:outline-none hover:bg-lightblue"
          aria-label="Back"
        >
          <i class="fas fa-arrow-left text-blue"></i>
        </button>
        <h1 class="text-xl font-bold ml-4">Tweet</h1>
      </div>

      <Loader :loading="loading" />

      <div v-if="!loading && notFound" class="flex flex-col items-center justify-center pt-10">
        <p class="font-bold text-lg">Tweet not found</p>
        <p class="text-sm text-dark">It may have been deleted or you don't have access.</p>
      </div>

      <div v-if="!loading && tweet && !notFound">
        <TweetBlock :tweet="tweet" />
        <div v-if="replies.length > 0" class="border-t border-lighter">
          <div class="px-5 py-3 text-sm text-dark">Replies</div>
          <TweetBlock v-for="r in replies" :key="r.id" :tweet="r" />
        </div>
      </div>
    </div>
    <DefaultRightBar :profile="ownerProfile" />
  </div>
</template>

<script>
import {defineAsyncComponent} from "vue";
import {warpnetService} from "@/service/service";

export default {
  name: "Tweet",
  components: {
    SideNav: defineAsyncComponent(() => import('@/components/SideNav.vue')),
    DefaultRightBar: defineAsyncComponent(() => import('@/components/DefaultRightBar.vue')),
    Loader: defineAsyncComponent(() => import('@/components/Loader.vue')),
    TweetBlock: defineAsyncComponent(() => import('@/components/TweetBlock.vue')),
  },
  data() {
    return {
      loading: true,
      notFound: false,
      tweet: null,
      replies: [],
      ownerProfile: {},
    };
  },
  async created() {
    console.log("loading component:", this.$options.name);
    this.ownerProfile = warpnetService.getOwnerProfile();
    const tweetId = this.$route.params.id;
    const userIdHint = this.$route.query.u || this.ownerProfile?.user_id;
    if (!tweetId) {
      this.notFound = true;
      this.loading = false;
      return;
    }
    try {
      this.tweet = await warpnetService.getTweet({userId: userIdHint, tweetId});
      if (!this.tweet || this.tweet.code) {
        this.notFound = true;
        this.loading = false;
        return;
      }
      const root = this.tweet.root_id || this.tweet.id;
      const repliesPage = await warpnetService.getReplies({
        rootId: root,
        parentId: this.tweet.id,
        cursorReset: true,
      });
      this.replies = Array.isArray(repliesPage) ? repliesPage : (repliesPage?.replies || []);
    } catch (err) {
      console.error('Failed to load tweet:', err);
      this.notFound = true;
    } finally {
      this.loading = false;
    }
  },
};
</script>
