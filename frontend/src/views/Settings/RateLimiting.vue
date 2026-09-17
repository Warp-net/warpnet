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
        <h1 class="text-xl font-bold ml-4">Rate limiting</h1>
      </div>

      <Loader :loading="loading" />

      <form v-if="!loading" @submit.prevent="save" class="p-5 space-y-4 max-w-xl">
        <p class="text-sm text-dark">
          How much traffic this node lets in. Raising the limits costs bandwidth,
          memory and CPU; lowering them makes the node drop peers that ask for too
          much. Changes take effect after the node restarts.
        </p>

        <fieldset class="border border-lighter rounded p-4 space-y-3">
          <legend class="font-bold px-1">Network</legend>
          <p class="text-sm text-dark">
            How many peers this node keeps connected. Above the high water mark the
            connection manager trims back down to the low one.
          </p>
          <label v-for="f in networkFields" :key="f.key" class="block">
            <span class="font-bold">{{ f.label }}</span>
            <input
              type="number" min="1" v-model.number="settings[f.key]"
              class="mt-1 w-full rounded border border-lighter bg-white p-2"
            />
            <span class="text-sm text-dark">{{ f.hint }}</span>
          </label>
        </fieldset>

        <fieldset class="border border-lighter rounded p-4 space-y-3">
          <legend class="font-bold px-1">Discovery</legend>
          <p class="text-sm text-dark">
            What one IP address may spend announcing peers, so that a flooding peer
            cannot shed the peers everyone else announces.
          </p>
          <label v-for="f in discoveryFields" :key="f.key" class="block">
            <span class="font-bold">{{ f.label }}</span>
            <input
              type="number" min="1" v-model.number="settings[f.key]"
              class="mt-1 w-full rounded border border-lighter bg-white p-2"
            />
            <span class="text-sm text-dark">{{ f.hint }}</span>
          </label>
        </fieldset>

        <fieldset class="border border-lighter rounded p-4 space-y-3">
          <legend class="font-bold px-1">Streams</legend>
          <p class="text-sm text-dark">
            What one peer may spend on this node's stream routes. Routes with a limit
            of their own — media, uploads, chats, pairing — keep it.
          </p>
          <label v-for="f in streamFields" :key="f.key" class="block">
            <span class="font-bold">{{ f.label }}</span>
            <input
              type="number" min="1" v-model.number="settings[f.key]"
              class="mt-1 w-full rounded border border-lighter bg-white p-2"
            />
            <span class="text-sm text-dark">{{ f.hint }}</span>
          </label>
        </fieldset>

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
            @click="settings = { ...DEFAULT_RATE_LIMITS }"
            class="text-blue font-semibold hover:underline"
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

const DEFAULT_RATE_LIMITS = {
  network_low_water: 20,
  network_high_water: 50,
  discovery_burst: 32,
  discovery_per_ten_sec: 2,
  stream_read_burst: 60,
  stream_read_per_minute: 300,
  stream_write_burst: 30,
  stream_write_per_minute: 120,
};

export default {
  name: "SettingsRateLimiting",
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
      networkFields: [
        { key: 'network_low_water', label: 'Low water mark', hint: 'Connections kept after trimming' },
        { key: 'network_high_water', label: 'High water mark', hint: 'Connections that start the trimming' },
      ],
      discoveryFields: [
        { key: 'discovery_burst', label: 'Burst', hint: 'Discoveries one IP may fire at once' },
        { key: 'discovery_per_ten_sec', label: 'Per 10 seconds', hint: 'How fast that budget refills' },
      ],
      streamFields: [
        { key: 'stream_read_burst', label: 'Read burst', hint: 'Reads one peer may fire at once' },
        { key: 'stream_read_per_minute', label: 'Reads per minute', hint: 'How fast the read budget refills' },
        { key: 'stream_write_burst', label: 'Write burst', hint: 'Writes one peer may fire at once' },
        { key: 'stream_write_per_minute', label: 'Writes per minute', hint: 'How fast the write budget refills' },
      ],
      settings: { ...DEFAULT_RATE_LIMITS },
      DEFAULT_RATE_LIMITS,
    };
  },
  methods: {
    async save() {
      this.savedMessage = '';
      this.saveError = false;
      if (Number(this.settings.network_low_water) >= Number(this.settings.network_high_water)) {
        this.savedMessage = 'Low water mark must be below the high one';
        this.saveError = true;
        return;
      }
      this.saving = true;
      try {
        const saved = await warpnetService.updateRateLimitSettings(this.settings);
        this.settings = { ...this.settings, ...saved };
        this.savedMessage = 'Settings saved';
      } catch (err) {
        console.error('Failed to save rate limit settings:', err);
        this.savedMessage = 'Failed to save';
        this.saveError = true;
        toast.error(err?.message || 'Failed to save rate limit settings.');
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
      const saved = await warpnetService.getRateLimitSettings();
      if (saved && typeof saved === 'object') {
        this.settings = { ...DEFAULT_RATE_LIMITS, ...saved };
      }
    } catch (err) {
      console.error('Failed to load rate limit settings:', err);
    } finally {
      this.loading = false;
    }
  },
};
</script>
