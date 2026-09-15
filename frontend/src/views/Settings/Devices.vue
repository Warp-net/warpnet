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
        <h1 class="text-xl font-bold ml-4">Devices</h1>
      </div>

      <div class="px-5 py-4 border-b border-lighter">
        <p class="text-sm text-dark">
          Phones paired with this node by scanning its QR code. A device may read and
          post as you until you remove it here, or until it stops checking in for
          three days. Up to {{ MAX_DEVICES }} devices can stay paired at once.
        </p>
        <p class="text-sm text-dark mt-2">
          A removed device loses access at once and cannot pair itself back on its
          own, whatever it still holds. Scanning this node's QR code on it again
          pairs it as a new device.
        </p>
      </div>

      <Loader :loading="loading" />

      <div
        v-if="!loading && failed"
        class="px-5 py-6"
      >
        <p class="font-bold text-lg">Devices are not available</p>
        <p class="text-sm text-dark">This node could not read its paired devices.</p>
      </div>

      <div
        v-if="!loading && !failed && devices.length === 0"
        class="flex flex-col items-center justify-center pt-10 px-5"
      >
        <p class="font-bold text-lg">No paired devices</p>
        <p class="text-sm text-dark text-center">
          Pair a phone from the mobile app by scanning this node's QR code.
        </p>
      </div>

      <div
        v-for="device in devices"
        :key="device.node_id"
        class="px-5 py-4 border-b border-lighter flex items-center"
      >
        <i class="fas fa-mobile-alt fa-fw text-2xl text-dark mr-4" aria-hidden="true"></i>
        <div class="min-w-0">
          <p class="font-bold">{{ platformLabel(device.platform) }}</p>
          <p class="text-sm text-dark font-mono truncate">{{ device.node_id }}</p>
          <p class="text-sm text-dark">
            Paired {{ formatTime(device.created_at) }} &middot;
            last seen {{ formatTime(device.last_active) }}
          </p>
        </div>
        <button
          @click="askRemove(device)"
          :disabled="removing === device.node_id"
          class="ml-auto shrink-0 text-red-600 hover:bg-red-50 rounded-full px-4 py-1 disabled:opacity-50"
        >
          {{ removing === device.node_id ? 'Removing…' : 'Remove' }}
        </button>
      </div>
    </div>
    <DefaultRightBar :profile="ownerProfile" />

    <ConfirmDialog
      :show="!!pendingDevice"
      title="Remove device"
      :message="confirmMessage"
      confirm-label="Remove"
      destructive
      @confirm="removeDevice"
      @cancel="pendingDevice = null"
    />
  </div>
</template>

<script>
import {defineAsyncComponent} from "vue";
import {warpnetService} from "@/service/service";
import {toast} from "@/lib/toast";

// Mirror of database.MaxAliases — shown so the user knows why pairing a
// further device is refused.
const MAX_DEVICES = 10;

export default {
  name: "SettingsDevices",
  components: {
    SideNav: defineAsyncComponent(() => import('@/components/SideNav.vue')),
    DefaultRightBar: defineAsyncComponent(() => import('@/components/DefaultRightBar.vue')),
    Loader: defineAsyncComponent(() => import('@/components/Loader.vue')),
    ConfirmDialog: defineAsyncComponent(() => import('@/components/ConfirmDialog.vue')),
  },
  data() {
    return {
      loading: true,
      failed: false,
      ownerProfile: {},
      devices: [],
      pendingDevice: null,
      removing: '',
      MAX_DEVICES,
    };
  },
  computed: {
    confirmMessage() {
      if (!this.pendingDevice) return '';
      return 'This device loses access to your node immediately. To pair it again, ' +
        `scan this node's QR code on it once more.\n\n${this.pendingDevice.node_id}`;
    },
  },
  methods: {
    platformLabel(platform) {
      return platform || 'Mobile device';
    },
    // A device paired by an older node carries no timestamps, and Go
    // sends those as year one rather than omitting them.
    formatTime(value) {
      if (!value) return 'never';
      const date = new Date(value);
      if (isNaN(date.getTime()) || date.getFullYear() < 2000) return 'never';
      return date.toLocaleString();
    },
    askRemove(device) {
      this.pendingDevice = device;
    },
    async removeDevice() {
      const device = this.pendingDevice;
      this.pendingDevice = null;
      if (!device) return;

      this.removing = device.node_id;
      try {
        this.devices = await warpnetService.deleteDevice(device.node_id);
      } catch (err) {
        console.error('Failed to remove device:', err);
        toast.error(err?.message || "Couldn't remove the device. Please try again.");
      } finally {
        this.removing = '';
      }
    },
  },
  async created() {
    this.ownerProfile = warpnetService.getOwnerProfile();
    try {
      this.devices = await warpnetService.getDevices();
    } catch (err) {
      console.error('Failed to load paired devices:', err);
      this.failed = true;
    } finally {
      this.loading = false;
    }
  },
};
</script>
