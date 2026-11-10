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
        <h1 class="text-xl font-bold ml-4">Wallet backend</h1>
      </div>

      <Loader :loading="loading" />

      <form v-if="!loading" @submit.prevent="save" class="p-5 space-y-4 max-w-xl">
        <p class="text-sm text-dark">
          Which blockchain API and payment engine the wallet uses. Changes take
          effect after the node restarts.
        </p>

        <label v-for="f in fields" :key="f.key" class="block">
          <span class="font-bold">{{ f.label }}</span>
          <input
            type="text" v-model.trim="settings[f.key]"
            class="mt-1 w-full rounded border border-lighter bg-white p-2"
          />
          <span class="text-sm text-dark">{{ f.hint }}</span>
        </label>

        <div class="flex items-center space-x-4">
          <button
            type="submit"
            :disabled="saving"
            class="text-white bg-blue rounded-full font-semibold px-5 py-2 hover:bg-darkblue disabled:opacity-50"
          >
            {{ saving ? 'Saving…' : 'Save' }}
          </button>
          <button
            type="button"
            :disabled="saving"
            @click="reset"
            class="text-blue font-semibold hover:underline disabled:opacity-50"
          >Reset to defaults</button>
        </div>
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
  name: "SettingsWallet",
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
      fields: [
        { key: 'endpoint', label: 'Blockchain API endpoint', hint: 'TronGrid-compatible http(s) URL' },
        { key: 'binary_path', label: 'Payment engine binary', hint: 'Leave empty to use the bundled one' },
      ],
      settings: {},
    };
  },
  methods: {
    async send(settings, message) {
      this.savedMessage = '';
      this.saveError = false;
      this.saving = true;
      try {
        this.settings = { ...(await warpnetService.updateWalletSettings(settings)) };
        this.savedMessage = message;
      } catch (err) {
        console.error('Failed to save wallet settings:', err);
        this.savedMessage = 'Failed to save';
        this.saveError = true;
        toast.error(err?.message || 'Failed to save wallet settings.');
      } finally {
        this.saving = false;
        if (this._savedTimer) clearTimeout(this._savedTimer);
        this._savedTimer = setTimeout(() => { this.savedMessage = ''; }, 3000);
      }
    },
    save() {
      return this.send(this.settings, 'Settings saved');
    },
    reset() {
      return this.send({}, 'Defaults restored');
    },
  },
  beforeUnmount() {
    if (this._savedTimer) clearTimeout(this._savedTimer);
  },
  async created() {
    this.ownerProfile = warpnetService.getOwnerProfile();
    try {
      const saved = await warpnetService.getWalletSettings();
      if (saved && typeof saved === 'object') {
        this.settings = { ...saved };
      }
    } catch (err) {
      console.error('Failed to load wallet settings:', err);
    } finally {
      this.loading = false;
    }
  },
};
</script>
