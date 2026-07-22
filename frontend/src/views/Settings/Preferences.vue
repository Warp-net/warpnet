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
        <h1 class="text-xl font-bold ml-4">Preferences</h1>
      </div>

      <Loader :loading="loading" />

      <form v-if="!loading" @submit.prevent="save" class="p-5 space-y-4 max-w-xl">
        <label class="block">
          <span class="font-bold">Default visibility</span>
          <select v-model="prefs.privacy" class="mt-1 w-full rounded border border-lighter bg-white p-2">
            <option value="public">Public</option>
            <option value="unlisted">Unlisted</option>
            <option value="private">Followers only</option>
            <option value="direct">Direct</option>
          </select>
        </label>
        <label class="flex items-center">
          <input type="checkbox" v-model="prefs.sensitive" class="mr-2" />
          <span class="font-bold">Mark media as sensitive by default</span>
        </label>
        <label class="block">
          <span class="font-bold">Default language</span>
          <input
            type="text"
            v-model="prefs.language"
            placeholder="en"
            maxlength="8"
            class="mt-1 w-full rounded border border-lighter bg-white p-2"
          />
          <span class="text-sm text-dark">ISO 639 code (e.g. en, ru, de)</span>
        </label>

        <button
          type="submit"
          :disabled="saving"
          class="text-white bg-blue rounded-full font-semibold px-5 py-2 hover:bg-darkblue disabled:opacity-50"
        >
          {{ saving ? 'Saving…' : 'Save' }}
        </button>
        <p v-if="savedMessage" class="text-sm font-medium" :class="saveError ? 'text-red-600' : 'text-green-700'">
          <i :class="saveError ? 'fas fa-exclamation-circle' : 'fas fa-check-circle'" aria-hidden="true"></i>
          {{ savedMessage }}
        </p>
      </form>
    </div>
    <DefaultRightBar :profile="ownerProfile" />
  </div>
</template>

<script>
import {defineAsyncComponent} from "vue";
import {warpnetService} from "@/service/service";
import {toast} from "@/lib/toast";

export default {
  name: "SettingsPreferences",
  components: {
    SideNav: defineAsyncComponent(() => import('@/components/SideNav.vue')),
    DefaultRightBar: defineAsyncComponent(() => import('@/components/DefaultRightBar.vue')),
    Loader: defineAsyncComponent(() => import('@/components/Loader.vue')),
  },
  data() {
    return {
      loading: true,
      saving: false,
      savedMessage: '',
      saveError: false,
      ownerProfile: {},
      prefs: { privacy: 'public', sensitive: false, language: 'en' },
    };
  },
  methods: {
    async save() {
      this.saving = true;
      this.savedMessage = '';
      this.saveError = false;
      try {
        await warpnetService.updateAccountSource(this.prefs);
        this.savedMessage = 'Saved';
      } catch (err) {
        console.error('Failed to save preferences:', err);
        this.savedMessage = 'Failed to save';
        this.saveError = true;
        toast.error(err?.message || 'Failed to save preferences. Please try again.');
      } finally {
        this.saving = false;
        if (this._savedTimer) clearTimeout(this._savedTimer);
        this._savedTimer = setTimeout(() => { this.savedMessage = ''; }, 3000);
      }
    },
  },
  beforeUnmount() {
    if (this._savedTimer) clearTimeout(this._savedTimer);
  },
  async created() {
    this.ownerProfile = warpnetService.getOwnerProfile();
    try {
      const profile = await warpnetService.getProfile(this.ownerProfile?.user_id);
      const meta = profile?.metadata;
      if (meta) {
        this.prefs.privacy = meta.privacy || 'public';
        this.prefs.sensitive = meta.sensitive === 'true';
        this.prefs.language = meta.language || 'en';
      }
    } catch (err) {
      console.error('Failed to load preferences:', err);
    } finally {
      this.loading = false;
    }
  },
};
</script>
