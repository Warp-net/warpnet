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

        <div class="border-t border-lighter p-3 flex flex-col gap-2">
          <textarea
            v-model="replyText"
            rows="2"
            maxlength="2000"
            placeholder="Tweet your reply"
            class="w-full rounded border border-lighter bg-white p-2 focus:outline-none focus:ring-2 focus:ring-blue text-sm"
          ></textarea>
          <div class="flex justify-end">
            <button
              @click="postReply"
              :disabled="posting || !replyText.trim()"
              class="text-white bg-blue rounded-full font-semibold px-4 py-1 hover:bg-darkblue disabled:opacity-50"
            >{{ posting ? 'Posting…' : 'Reply' }}</button>
          </div>
        </div>

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
      replyText: '',
      posting: false,
    };
  },
  methods: {
    async loadTweet() {
      const tweetId = this.$route.params.id;
      const userIdHint = this.$route.query.u || this.ownerProfile?.user_id;
      const parentId = this.$route.query.parent || '';
      const rootId = this.$route.query.root || '';
      if (!tweetId) {
        this.notFound = true;
        return;
      }

      // Replies live in the thread index, not under the user's TWEETS
      // partition, so they're fetched by their parent (the tweet they reply
      // to) when the route hint says we're looking at a reply.
      const isReply = !!parentId && rootId && rootId !== tweetId;
      let fetched = null;
      if (isReply) {
        fetched = await warpnetService.getReply({
          parentId, replyId: tweetId, userId: userIdHint,
        });
        if (fetched && fetched.reply) fetched = fetched.reply;
      } else {
        fetched = await warpnetService.getTweet({userId: userIdHint, tweetId});
      }
      if (!fetched || fetched.code || !fetched.id) {
        this.notFound = true;
        return;
      }
      this.tweet = fetched;

      const root = this.tweet.root_id || this.tweet.id;
      const repliesPage = await warpnetService.getReplies({
        rootId: root,
        parentId: this.tweet.id,
        // user_id is the root author only when this tweet is the root itself.
        rootUserId: root === this.tweet.id ? this.tweet.user_id : undefined,
        cursorReset: true,
      });
      this.replies = Array.isArray(repliesPage) ? repliesPage : (repliesPage?.replies || []);
    },
    async postReply() {
      const text = this.replyText.trim();
      if (!text || this.posting) return;
      this.posting = true;
      try {
        const root = this.tweet.root_id || this.tweet.id;
        const created = await warpnetService.replyTweet({
          rootId: root,
          parentId: this.tweet.id,
          parentUserId: this.tweet.user_id,
          text,
        });
        if (created && created.id) {
          this.replies = [created, ...this.replies];
        }
        this.replyText = '';
      } catch (err) {
        console.error('Failed to post reply:', err);
      } finally {
        this.posting = false;
      }
    },
  },
  async created() {
    console.log("loading component:", this.$options.name);
    this.ownerProfile = warpnetService.getOwnerProfile();
    try {
      await this.loadTweet();
    } catch (err) {
      console.error('Failed to load tweet:', err);
      this.notFound = true;
    } finally {
      this.loading = false;
    }
  },
};
</script>
