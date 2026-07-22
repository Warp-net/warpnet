<!-- Warpnet - Decentralized Social Network -->
<template>
  <div
    v-if="show"
    class="fixed inset-0 bg-black bg-opacity-50 z-50 flex items-center justify-center p-4"
    @click.self.stop="$emit('close')"
    @click.stop
  >
    <div class="bg-white dark:bg-darktheme-card mastodon:bg-mastodon-card rounded-lg w-full max-w-lg max-h-[80vh] flex flex-col">
      <div class="px-5 py-3 border-b border-lighter flex items-center">
        <h2 class="font-bold text-lg">Notification</h2>
        <button
          @click="$emit('close')"
          class="ml-auto text-dark hover:text-black"
          aria-label="Close"
        >
          <i class="fas fa-times"></i>
        </button>
      </div>
      <div class="overflow-y-auto flex-1 p-5">
        <Loader :loading="loading" />
        <div v-if="!loading && !notification" class="text-center text-dark">Not found</div>
        <div v-if="!loading && notification">
          <div class="flex items-start mb-3">
            <i :class="['mr-3 mt-1', icon]" aria-hidden="true"></i>
            <div>
              <p class="font-bold">{{ notification.text }}</p>
              <p class="text-sm text-dark">{{ $filters.timeago(notification.created_at) }}</p>
            </div>
          </div>
          <div v-if="notification.tweet_id" class="border-t border-lighter pt-3">
            <button
              @click="openTweet"
              class="text-blue hover:underline"
            >
              View tweet
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import {defineAsyncComponent} from "vue";
import {warpnetService} from "@/service/service";
import {dismissable} from "@/lib/modal.mixin";

const iconByType = {
  reply: 'fas fa-comment text-blue',
  like: 'fas fa-heart text-red-600',
  retweet: 'fas fa-retweet text-green-500',
  follow: 'fas fa-user-plus text-blue',
  mention: 'fas fa-at text-blue',
  moderation: 'fas fa-shield-alt text-blue',
  message: 'fas fa-envelope text-blue',
};

export default {
  name: "NotificationOverlay",
  mixins: [dismissable("close")],
  components: {
    Loader: defineAsyncComponent(() => import('@/components/Loader.vue')),
  },
  props: {
    show: { type: Boolean, default: false },
    notificationId: { type: String, default: '' },
  },
  emits: ['close'],
  data() {
    return { loading: true, notification: null };
  },
  computed: {
    icon() {
      return iconByType[this.notification?.type] || 'fas fa-bell text-dark';
    },
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
    openTweet() {
      if (!this.notification?.tweet_id) return;
      this.$emit('close');
      this.$router.push({
        name: 'Tweet',
        params: { id: this.notification.tweet_id },
        query: { u: this.notification.user_id || '' },
      });
    },
    async load() {
      if (!this.notificationId) {
        this.notification = null;
        this.loading = false;
        return;
      }
      this.loading = true;
      try {
        this.notification = await warpnetService.getNotification(this.notificationId);
      } catch (err) {
        console.error('Failed to load notification:', err);
        this.notification = null;
      } finally {
        this.loading = false;
      }
    },
  },
};
</script>
