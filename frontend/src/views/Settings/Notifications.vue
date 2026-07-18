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
        <h1 class="text-xl font-bold ml-4">Email notifications</h1>
      </div>

      <Loader :loading="loading" />

      <form v-if="!loading" @submit.prevent="save" class="p-5 space-y-4 max-w-xl">
        <label class="flex items-center">
          <input type="checkbox" v-model="settings.email_enabled" class="mr-2" />
          <span class="font-bold">Enable email notifications</span>
        </label>

        <label class="block">
          <span class="font-bold">Recipient email</span>
          <input
            type="email"
            v-model="settings.recipient"
            placeholder="you@example.com"
            class="mt-1 w-full rounded border border-lighter bg-white p-2"
          />
        </label>

        <fieldset class="border border-lighter rounded p-4 space-y-3">
          <legend class="font-bold px-1">SMTP server</legend>
          <p class="text-sm text-dark">
            Warpnet sends email through your own SMTP account. Credentials are stored
            only on this device, inside the encrypted local database.
          </p>
          <label class="block">
            <span class="font-bold">Host</span>
            <input type="text" v-model="settings.smtp_host" placeholder="smtp.example.com"
                   class="mt-1 w-full rounded border border-lighter bg-white p-2" />
          </label>
          <label class="block">
            <span class="font-bold">Port</span>
            <input type="number" v-model.number="settings.smtp_port" placeholder="587"
                   class="mt-1 w-full rounded border border-lighter bg-white p-2" />
          </label>
          <label class="block">
            <span class="font-bold">Username</span>
            <input type="text" v-model="settings.smtp_username" autocomplete="off"
                   class="mt-1 w-full rounded border border-lighter bg-white p-2" />
          </label>
          <label class="block">
            <span class="font-bold">Password</span>
            <input type="password" v-model="settings.smtp_password" autocomplete="new-password"
                   class="mt-1 w-full rounded border border-lighter bg-white p-2" />
          </label>
          <label class="block">
            <span class="font-bold">From address</span>
            <input type="email" v-model="settings.smtp_from" placeholder="you@example.com"
                   class="mt-1 w-full rounded border border-lighter bg-white p-2" />
          </label>
          <label class="flex items-center">
            <input type="checkbox" v-model="settings.smtp_use_tls" class="mr-2" />
            <span class="font-bold">Use implicit TLS (port 465)</span>
          </label>
        </fieldset>

        <fieldset class="border border-lighter rounded p-4 space-y-2">
          <legend class="font-bold px-1">Email me about</legend>
          <label v-for="t in types" :key="t.key" class="flex items-center">
            <input type="checkbox" v-model="settings.types[t.key]" class="mr-2" />
            <span>{{ t.label }}</span>
          </label>
        </fieldset>

        <button
          type="submit"
          :disabled="saving"
          class="text-white bg-blue rounded-full font-semibold px-5 py-2 hover:bg-darkblue disabled:opacity-50"
        >
          {{ saving ? 'Saving…' : 'Save' }}
        </button>
        <p v-if="savedMessage" class="text-sm text-dark">{{ savedMessage }}</p>
      </form>
    </div>
    <DefaultRightBar :profile="ownerProfile" />
  </div>
</template>

<script>
import {defineAsyncComponent} from "vue";
import {warpnetService} from "@/service/service";

export default {
  name: "SettingsNotifications",
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
      ownerProfile: {},
      types: [
        { key: 'follow', label: 'New followers' },
        { key: 'like', label: 'Likes on your tweets' },
        { key: 'retweet', label: 'Retweets and quotes' },
        { key: 'reply', label: 'Replies to your tweets' },
        { key: 'message', label: 'Direct messages' },
        { key: 'moderation', label: 'Moderation results' },
        { key: 'new_user', label: 'New user discovered' },
      ],
      settings: {
        email_enabled: false,
        recipient: '',
        smtp_host: '',
        smtp_port: 587,
        smtp_username: '',
        smtp_password: '',
        smtp_from: '',
        smtp_use_tls: false,
        types: {},
      },
    };
  },
  methods: {
    async save() {
      this.saving = true;
      this.savedMessage = '';
      try {
        await warpnetService.updateNotificationSettings(this.settings);
        this.savedMessage = 'Saved';
      } catch (err) {
        console.error('Failed to save notification settings:', err);
        this.savedMessage = 'Failed to save';
      } finally {
        this.saving = false;
      }
    },
  },
  async created() {
    this.ownerProfile = warpnetService.getOwnerProfile();
    try {
      const saved = await warpnetService.getNotificationSettings();
      if (saved && typeof saved === 'object') {
        this.settings = {
          email_enabled: !!saved.email_enabled,
          recipient: saved.recipient || '',
          smtp_host: saved.smtp_host || '',
          smtp_port: saved.smtp_port || 587,
          smtp_username: saved.smtp_username || '',
          smtp_password: saved.smtp_password || '',
          smtp_from: saved.smtp_from || '',
          smtp_use_tls: !!saved.smtp_use_tls,
          types: saved.types || {},
        };
      }
    } catch (err) {
      console.error('Failed to load notification settings:', err);
    } finally {
      this.loading = false;
    }
  },
};
</script>
