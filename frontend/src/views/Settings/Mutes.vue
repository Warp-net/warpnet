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
          @click="$router.push({ name: 'Settings' })"
          class="rounded-full md:pr-2 focus:outline-none hover:bg-lightblue"
          aria-label="Back"
        >
          <i class="fas fa-arrow-left text-blue"></i>
        </button>
        <h1 class="text-xl font-bold ml-4">Muted users</h1>
      </div>

      <Loader :loading="loading" />

      <div
        v-if="!loading && mutedUsers.length === 0"
        class="flex flex-col items-center justify-center pt-10 px-5"
      >
        <p class="font-bold text-lg">You haven't muted anyone yet</p>
        <p class="text-sm text-dark">Mute to hide a user's tweets and replies from your timelines without blocking them.</p>
      </div>

      <div v-for="user in mutedUsers" :key="user.id" class="px-5 py-3 border-b border-lighter flex items-center">
        <img
          :src="user.avatar || '/default_profile.png'"
          class="h-10 w-10 rounded-full object-cover"
          alt=""
        />
        <button
          @click="$router.push({ name: 'Profile', params: { id: user.id } })"
          class="ml-3 text-left flat-btn"
        >
          <p class="font-bold">{{ user.username || user.id }}</p>
          <p class="text-dark text-sm">@{{ user.id }}</p>
        </button>
        <button
          @click="unmute(user.id)"
          class="ml-auto text-blue border border-blue font-bold px-3 py-1 rounded-full hover:bg-lightblue"
        >
          Unmute
        </button>
      </div>
    </div>
    <DefaultRightBar :profile="ownerProfile" />
  </div>
</template>

<script>
import {defineAsyncComponent} from "vue";
import {warpnetService} from "@/service/service";
import {toast} from "@/lib/toast";

export default {
  name: "SettingsMutes",
  components: {
    SideNav: defineAsyncComponent(() => import('@/components/SideNav.vue')),
    DefaultRightBar: defineAsyncComponent(() => import('@/components/DefaultRightBar.vue')),
    Loader: defineAsyncComponent(() => import('@/components/Loader.vue')),
  },
  data() {
    return {
      loading: true,
      mutedUsers: [],
      done: false,
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
            if (p.avatar_key) {
              p.avatar = await warpnetService.getImage({userId: id, key: p.avatar_key});
            }
            return p;
          } catch (e) { return { id }; }
        })
      );
    },
    async loadMore() {
      if (this.done || this.loading) return;
      const resp = await warpnetService.getMutes(false);
      const ids = resp?.ids || [];
      if (ids.length === 0) { this.done = true; return; }
      const users = await this.hydrate(ids);
      this.mutedUsers = this.mutedUsers.concat(users);
    },
    async unmute(id) {
      try {
        await warpnetService.unmuteUser(id);
        this.mutedUsers = this.mutedUsers.filter(u => u.id !== id);
      } catch (err) {
        console.error('Failed to unmute', id, err);
        toast.error(err?.message || "Couldn't unmute this user. Please try again.");
      }
    },
  },
  async created() {
    this.ownerProfile = warpnetService.getOwnerProfile();
    try {
      const resp = await warpnetService.getMutes(true);
      const ids = resp?.ids || [];
      this.mutedUsers = await this.hydrate(ids);
      if (resp?.cursor === 'end') this.done = true;
    } catch (err) {
      console.error('Failed to load mutes:', err);
    } finally {
      this.loading = false;
    }
  },
};
</script>
