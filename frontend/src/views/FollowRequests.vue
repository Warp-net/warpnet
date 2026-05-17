<!-- Warpnet - Decentralized Social Network -->
<template>
  <div id="app" class="flex container h-screen w-full">
    <SideNav />
    <div class="w-full h-full overflow-y-scroll no-scrollbar">
      <div class="px-5 py-3 border-b border-lighter flex items-center">
        <button
          @click="$router.push({ name: 'Settings' })"
          class="rounded-full md:pr-2 focus:outline-none hover:bg-lightblue"
          aria-label="Back"
        >
          <i class="fas fa-arrow-left text-blue"></i>
        </button>
        <h1 class="text-xl font-bold ml-4">Follow requests</h1>
      </div>

      <Loader :loading="loading" />

      <div
        v-if="!loading && requests.length === 0"
        class="flex flex-col items-center justify-center pt-10 px-5"
      >
        <p class="font-bold text-lg">No follow requests</p>
        <p class="text-sm text-dark">When your account is locked, requests to follow you appear here.</p>
      </div>

      <div v-for="r in requests" :key="r.id" class="px-5 py-3 border-b border-lighter flex items-center">
        <img :src="r.avatar || '/default_profile.png'" class="h-10 w-10 rounded-full object-cover" alt="" />
        <button @click="$router.push({ name: 'Profile', params: { id: r.id } })" class="ml-3 text-left flat-btn">
          <p class="font-bold">{{ r.username || r.id }}</p>
          <p class="text-dark text-sm">@{{ r.id }}</p>
        </button>
        <div class="ml-auto flex gap-2">
          <button
            @click="authorize(r.id)"
            class="text-white bg-blue font-bold px-3 py-1 rounded-full hover:bg-darkblue"
          >Authorize</button>
          <button
            @click="reject(r.id)"
            class="text-red-600 border border-red-600 font-bold px-3 py-1 rounded-full hover:bg-red-50"
          >Reject</button>
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
  name: "FollowRequests",
  components: {
    SideNav: defineAsyncComponent(() => import('@/components/SideNav.vue')),
    DefaultRightBar: defineAsyncComponent(() => import('@/components/DefaultRightBar.vue')),
    Loader: defineAsyncComponent(() => import('@/components/Loader.vue')),
  },
  data() {
    return {
      loading: true,
      requests: [],
      ownerProfile: {},
    };
  },
  methods: {
    async hydrate(ids) {
      return Promise.all(
        (ids || []).map(async (id) => {
          try {
            const p = await warpnetService.getProfile(id);
            if (!p) return { id };
            if (p.avatar_key) p.avatar = await warpnetService.getImage({userId: id, key: p.avatar_key});
            return p;
          } catch (e) { return { id }; }
        })
      );
    },
    async authorize(id) {
      try {
        await warpnetService.authorizeFollowRequest(id);
        this.requests = this.requests.filter(r => r.id !== id);
      } catch (err) { console.error('Failed to authorize', err); }
    },
    async reject(id) {
      try {
        await warpnetService.rejectFollowRequest(id);
        this.requests = this.requests.filter(r => r.id !== id);
      } catch (err) { console.error('Failed to reject', err); }
    },
  },
  async created() {
    this.ownerProfile = warpnetService.getOwnerProfile();
    try {
      const resp = await warpnetService.getFollowRequests();
      const ids = resp?.follower_ids || resp?.ids || [];
      this.requests = await this.hydrate(ids);
    } catch (err) {
      console.error('Failed to load follow requests:', err);
    } finally {
      this.loading = false;
    }
  },
};
</script>
