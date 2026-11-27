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
        <h1 class="text-xl font-bold ml-4">Wallet</h1>
      </div>

      <Loader :loading="loading" />

      <form v-if="!loading" @submit.prevent="save" class="p-5 space-y-4 max-w-xl">
        <label class="flex items-start space-x-3">
          <input
            type="checkbox" v-model="settings.enabled" :disabled="saving"
            @change="toggle(settings.enabled)"
            class="mt-1" data-testid="wallet-enabled"
          />
          <span>
            <span class="font-bold">Use the wallet</span>
            <span class="block text-sm text-dark">
              Pay for sponsored tweets, sell your own and send TRX and USDT.
              While it is off the payment engine never starts, the Wallet tab is
              hidden and the node tells others it takes no payments. Your
              wallet is derived from your account, so turning it off loses nothing.
            </span>
          </span>
        </label>

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

        <label class="block">
          <span class="font-bold">Request headers</span>
          <textarea
            v-model="headers" rows="3" spellcheck="false" placeholder="TRON-PRO-API-KEY: your-key"
            class="mt-1 w-full rounded border border-lighter bg-white p-2 font-mono text-sm"
          ></textarea>
          <span class="text-sm text-dark">One "Name: value" per line, such as your provider's API key</span>
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
      headers: '',
    };
  },
  methods: {
    async send(settings, message) {
      let saved = false;
      this.savedMessage = '';
      this.saveError = false;
      this.saving = true;
      try {
        await warpnetService.updateWalletSettings(settings);
        this.load(await warpnetService.getWalletSettings());
        this.savedMessage = message;
        saved = true;
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
      return saved;
    },
    load(saved) {
      this.settings = { ...saved };
      this.headers = Object.entries(saved.headers || {}).map(([k, v]) => `${k}: ${v}`).join('\n');
    },
    formSettings() {
      const headers = Object.fromEntries(this.headers.split('\n')
        .map((line) => line.split(':'))
        .filter(([name, ...value]) => name.trim() && value.length)
        .map(([name, ...value]) => [name.trim(), value.join(':').trim()]));
      return { ...this.settings, headers };
    },
    save() {
      return this.send(this.formSettings(), 'Settings saved');
    },
    async toggle(enabled) {
      if (!await this.send({ ...this.formSettings(), enabled }, enabled ? 'Wallet turned on' : 'Wallet turned off')) {
        this.settings.enabled = !enabled;
      }
    },
    reset() {
      return this.send({ enabled: !!this.settings.enabled }, 'Defaults restored');
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
        this.load(saved);
      }
    } catch (err) {
      console.error('Failed to load wallet settings:', err);
    } finally {
      this.loading = false;
    }
  },
};
</script>
