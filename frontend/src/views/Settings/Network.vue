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
        <h1 class="text-xl font-bold ml-4">Network</h1>
      </div>

      <Loader :loading="loading" />

      <form v-if="!loading" @submit.prevent="save" class="p-5 space-y-4 max-w-xl">
        <p class="text-sm text-dark">
          Where this node listens and which peers it joins the network through.
          Changes take effect after the node restarts. A flag or an environment
          variable set at launch overrides the value saved here.
        </p>

        <fieldset class="border border-lighter rounded p-4 space-y-3">
          <legend class="font-bold px-1">Listen address</legend>
          <label v-for="f in listenFields" :key="f.key" class="block">
            <span class="font-bold">{{ f.label }}</span>
            <input
              type="text" :inputmode="f.inputmode" v-model.trim="settings[f.key]"
              class="mt-1 w-full rounded border border-lighter bg-white p-2"
            />
            <span class="text-sm text-dark">{{ f.hint }}</span>
          </label>
        </fieldset>

        <fieldset class="border border-lighter rounded p-4 space-y-3">
          <legend class="font-bold px-1">Bootstrap peers</legend>
          <p class="text-sm text-dark">
            The first peers this node connects to. The peer ID proves the node at
            that address is the one you meant. Paired phones get the same list.
          </p>
          <div
            v-for="(peer, i) in bootstrap" :key="peer.key"
            class="flex flex-wrap items-end gap-2 border-b border-lighter pb-3"
          >
            <label class="block flex-1 min-w-[8rem]">
              <span class="text-sm font-bold">Host</span>
              <input
                type="text" v-model.trim="peer.host" placeholder="1.2.3.4"
                class="mt-1 w-full rounded border border-lighter bg-white p-2"
              />
            </label>
            <label class="block w-24">
              <span class="text-sm font-bold">Port</span>
              <input
                type="text" inputmode="numeric" v-model.trim="peer.port" placeholder="4001"
                class="mt-1 w-full rounded border border-lighter bg-white p-2"
              />
            </label>
            <label class="block w-full">
              <span class="text-sm font-bold">Peer ID</span>
              <input
                type="text" v-model.trim="peer.peerId" placeholder="12D3KooW…" spellcheck="false"
                class="mt-1 w-full rounded border border-lighter bg-white p-2 font-mono text-sm"
              />
            </label>
            <button
              type="button" @click="bootstrap.splice(i, 1)"
              class="text-red-600 text-sm font-semibold hover:underline"
            >
              <i class="fas fa-trash-alt" aria-hidden="true"></i> Remove
            </button>
          </div>
          <button
            type="button" @click="addPeer"
            class="text-blue font-semibold hover:underline"
          >
            <i class="fas fa-plus" aria-hidden="true"></i> Add bootstrap peer
          </button>
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

const PEER_ADDR = /^\/(ip4|ip6|dns|dns4|dns6)\/([^/]+)\/tcp\/(\d+)\/p2p\/([^/]+)$/;
const IPV4 = /^\d{1,3}(\.\d{1,3}){3}$/;

let peerKey = 0;

function parsePeer(addr) {
  const m = PEER_ADDR.exec(addr);
  if (!m) return { key: ++peerKey, host: addr, port: '', peerId: '' };
  return { key: ++peerKey, host: m[2], port: m[3], peerId: m[4] };
}

function buildPeerAddr(peer) {
  let proto = 'dns';
  if (IPV4.test(peer.host)) proto = 'ip4';
  else if (peer.host.includes(':')) proto = 'ip6';
  return `/${proto}/${peer.host}/tcp/${peer.port}/p2p/${peer.peerId}`;
}

function isValidPort(value) {
  const port = Number(value);
  return Number.isInteger(port) && port >= 1 && port <= 65535;
}

export default {
  name: "SettingsNetwork",
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
      listenFields: [
        { key: 'host_v4', label: 'IPv4 host', hint: '0.0.0.0 listens on every IPv4 interface', inputmode: 'decimal' },
        { key: 'host_v6', label: 'IPv6 host', hint: ':: listens on every IPv6 interface', inputmode: 'text' },
        { key: 'port', label: 'Port', hint: 'TCP port for peer connections', inputmode: 'numeric' },
      ],
      settings: {},
      bootstrap: [],
    };
  },
  methods: {
    apply(saved) {
      this.settings = { ...saved };
      this.bootstrap = (saved.bootstrap || []).map(parsePeer);
    },
    addPeer() {
      this.bootstrap.push({ key: ++peerKey, host: '', port: '4001', peerId: '' });
    },
    fail(message) {
      this.savedMessage = message;
      this.saveError = true;
    },
    async send(settings, message) {
      this.savedMessage = '';
      this.saveError = false;
      this.saving = true;
      try {
        this.apply(await warpnetService.updateNetworkSettings(settings));
        this.savedMessage = message;
      } catch (err) {
        console.error('Failed to save network settings:', err);
        this.fail('Failed to save');
        toast.error(err?.message || 'Failed to save network settings.');
      } finally {
        this.saving = false;
        if (this._savedTimer) clearTimeout(this._savedTimer);
        this._savedTimer = setTimeout(() => { this.savedMessage = ''; }, 3000);
      }
    },
    save() {
      if (!isValidPort(this.settings.port)) {
        return this.fail('Port must be between 1 and 65535');
      }
      const peers = this.bootstrap.filter((p) => p.host || p.peerId);
      const incomplete = peers.find((p) => !p.host || !p.peerId || !isValidPort(p.port));
      if (incomplete) {
        return this.fail('Every bootstrap peer needs a host, a port and a peer ID');
      }
      return this.send({ ...this.settings, bootstrap: peers.map(buildPeerAddr) }, 'Settings saved');
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
      const saved = await warpnetService.getNetworkSettings();
      if (saved && typeof saved === 'object') {
        this.apply(saved);
      }
    } catch (err) {
      console.error('Failed to load network settings:', err);
    } finally {
      this.loading = false;
    }
  },
};
</script>
