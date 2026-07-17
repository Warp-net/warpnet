<!-- Warpnet - Decentralized Social Network -->
<template>
  <div id="app" class="flex container h-screen w-full">
    <SideNav />
    <div class="w-full h-full overflow-y-scroll no-scrollbar">
      <div class="px-5 py-3 border-b border-lighter flex items-center">
        <button
          @click="$router.push({ name: 'Home' })"
          class="rounded-full md:pr-2 focus:outline-none hover:bg-lightblue"
          aria-label="Back"
        >
          <i class="fas fa-arrow-left text-blue"></i>
        </button>
        <h1 class="text-xl font-bold ml-4">Settings</h1>
      </div>
      <nav class="flex flex-col">
        <button
          v-for="item in items"
          :key="item.name"
          @click="$router.push({ name: item.name })"
          class="text-left px-5 py-4 border-b border-lighter hover:bg-lightest flex items-center"
        >
          <i :class="['fa-fw mr-4 text-dark', item.icon]" aria-hidden="true"></i>
          <div>
            <p class="font-bold">{{ item.label }}</p>
            <p class="text-sm text-dark">{{ item.hint }}</p>
          </div>
          <i class="fas fa-chevron-right ml-auto text-dark text-sm"></i>
        </button>
        <button
          @click="showImportModal = true"
          class="text-left px-5 py-4 border-b border-lighter hover:bg-lightest flex items-center"
        >
          <i class="fa-fw mr-4 text-dark fas fa-file-import" aria-hidden="true"></i>
          <div>
            <p class="font-bold">Import tweets from X</p>
            <p class="text-sm text-dark">Import your original tweets and photos from an X archive</p>
          </div>
          <i class="fas fa-chevron-right ml-auto text-dark text-sm"></i>
        </button>
      </nav>
    </div>
    <DefaultRightBar :profile="ownerProfile" />

    <ImportTweetsModal
        :show="showImportModal"
        @close="showImportModal = false"
    />
  </div>
</template>

<script>
import {defineAsyncComponent} from "vue";
import {warpnetService} from "@/service/service";

export default {
  name: "Settings",
  components: {
    SideNav: defineAsyncComponent(() => import('@/components/SideNav.vue')),
    DefaultRightBar: defineAsyncComponent(() => import('@/components/DefaultRightBar.vue')),
    ImportTweetsModal: defineAsyncComponent(() => import('@/components/ImportTweetsModal.vue')),
  },
  data() {
    return {
      ownerProfile: {},
      showImportModal: false,
      items: [
        { name: 'SettingsPreferences', label: 'Preferences', hint: 'Default visibility, language', icon: 'fas fa-sliders-h' },
        { name: 'SettingsBlocks', label: 'Blocked users', hint: 'Users you have blocked', icon: 'fas fa-ban' },
        { name: 'SettingsMutes', label: 'Muted users', hint: 'Users you have muted', icon: 'fas fa-volume-mute' },
        { name: 'SettingsFilters', label: 'Filters', hint: 'Hide tweets matching keywords', icon: 'fas fa-filter' },
        { name: 'SettingsNotifications', label: 'Email notifications', hint: 'Send notifications to your email', icon: 'fas fa-envelope' },
      ],
    };
  },
  async created() {
    this.ownerProfile = warpnetService.getOwnerProfile();
  },
};
</script>
