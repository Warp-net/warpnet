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
        <h1 class="text-xl font-bold ml-4">Fediverse gateway</h1>
      </div>

      <Loader :loading="loading" />

      <form v-if="!loading" @submit.prevent="save" class="p-5 space-y-4 max-w-xl">
        <p class="text-sm text-dark">
          Bridged Mastodon accounts resolve through the ActivityPub gateway node.
          Change its peer id only if you run your own gateway. Changes take effect
          after the node restarts.
        </p>

        <label class="block">
          <span class="font-bold">Gateway node id</span>
          <input
            type="text"
            v-model="nodeId"
            spellcheck="false"
            autocomplete="off"
            placeholder="12D3Koo…"
            class="mt-1 w-full rounded border border-lighter bg-white p-2 font-mono text-sm"
          />
        </label>

        <p class="text-sm text-dark">
          Default:
          <button
            type="button"
            @click="nodeId = DEFAULT_GATEWAY_NODE_ID"
            class="font-mono text-blue hover:underline break-all"
          >{{ DEFAULT_GATEWAY_NODE_ID }}</button>
        </p>

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

// Mirror of mastodon.DefaultGatewayNodeID; used as the reset value and as a
// fallback if the node response omits node_id.
const DEFAULT_GATEWAY_NODE_ID = "12D3KooWRyHvpYFjCzorxuSyXFigPfhYaHh1GW1JmwQJSPdmj4JK";

export default {
  name: "SettingsGateway",
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
      nodeId: DEFAULT_GATEWAY_NODE_ID,
      DEFAULT_GATEWAY_NODE_ID,
    };
  },
  methods: {
    async save() {
      this.saving = true;
      this.savedMessage = '';
      this.saveError = false;
      try {
        const resp = await warpnetService.updateGatewaySettings(this.nodeId);
        this.nodeId = resp.node_id || DEFAULT_GATEWAY_NODE_ID;
        this.savedMessage = 'Settings saved';
      } catch (err) {
        console.error('Failed to save gateway settings:', err);
        this.savedMessage = 'Failed to save';
        this.saveError = true;
        toast.error(err?.message || 'Failed to save gateway settings.');
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
      const saved = await warpnetService.getGatewaySettings();
      if (saved && typeof saved === 'object' && saved.node_id) {
        this.nodeId = saved.node_id;
      }
    } catch (err) {
      console.error('Failed to load gateway settings:', err);
    } finally {
      this.loading = false;
    }
  },
};
</script>
