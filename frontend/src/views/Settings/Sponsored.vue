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
        <h1 class="text-xl font-bold ml-4">Sponsored tweets</h1>
      </div>

      <Loader :loading="loading" />

      <form v-if="!loading" @submit.prevent="save" class="p-5 space-y-4 max-w-xl">
        <p class="text-sm text-dark">
          Sell your sponsored tweets only to wallets that keep staked TRX. Unstaking takes 14 days,
          so a buyer you blocked for leaking can't come back with a new wallet right away.
          A wallet that paid for one buyer can't pay for another. 0 asks nothing.
        </p>

        <label class="block">
          <span class="font-bold">Staked TRX a buyer must hold</span>
          <input
            type="number"
            v-model.number="minStakeTrx"
            min="0"
            step="1"
            class="mt-1 w-full rounded border border-lighter bg-white p-2 text-sm"
          />
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
  name: "SettingsSponsored",
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
      minStakeTrx: 0,
    };
  },
  methods: {
    async save() {
      this.saving = true;
      this.savedMessage = '';
      this.saveError = false;
      try {
        const resp = await warpnetService.updateSponsoredSettings(this.minStakeTrx);
        this.minStakeTrx = resp.min_stake_trx;
        this.savedMessage = 'Settings saved';
      } catch (err) {
        console.error('Failed to save sponsored settings:', err);
        this.savedMessage = 'Failed to save';
        this.saveError = true;
        toast.error(err?.message || 'Failed to save sponsored settings.');
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
      const saved = await warpnetService.getSponsoredSettings();
      this.minStakeTrx = Number(saved.min_stake_trx) || 0;
    } catch (err) {
      console.error('Failed to load sponsored settings:', err);
    } finally {
      this.loading = false;
    }
  },
};
</script>
