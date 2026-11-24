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
        <h1 class="text-xl font-bold ml-4">Screen lock</h1>
      </div>

      <div class="px-5 py-4 border-b border-lighter">
        <p class="text-sm text-dark">
          Hides this window behind a PIN while you are away. Your node keeps running,
          so paired phones stay connected. Lock it from the account menu or with
          Ctrl+L. After {{ MAX_UNLOCK_ATTEMPTS }} wrong PINs the app signs out and
          asks for your password.
        </p>
      </div>

      <form @submit.prevent="save" class="p-5 space-y-4 max-w-xl">
        <label v-if="screenLock.isEnabled" class="block">
          <span class="font-bold">Current PIN</span>
          <input v-model="currentPin" type="password" inputmode="numeric" autocomplete="off" maxlength="8"
                 class="mt-1 w-full rounded border border-lighter bg-white p-2" />
        </label>
        <label class="block">
          <span class="font-bold">{{ screenLock.isEnabled ? 'New PIN' : 'PIN' }}</span>
          <input v-model="newPin" type="password" inputmode="numeric" autocomplete="off" maxlength="8"
                 class="mt-1 w-full rounded border border-lighter bg-white p-2" />
          <span class="text-sm text-dark">
            4 to 8 digits{{ screenLock.isEnabled ? '; leave empty to keep the current one' : '' }}
          </span>
        </label>
        <label v-if="newPin" class="block">
          <span class="font-bold">Repeat PIN</span>
          <input v-model="newPinRepeat" type="password" inputmode="numeric" autocomplete="off" maxlength="8"
                 class="mt-1 w-full rounded border border-lighter bg-white p-2" />
        </label>
        <label class="block">
          <span class="font-bold">Lock after inactivity</span>
          <select v-model.number="idleMinutes" class="mt-1 w-full rounded border border-lighter bg-white p-2">
            <option v-for="minutes in IDLE_MINUTES_CHOICES" :key="minutes" :value="minutes">
              {{ minutes === 0 ? 'Never' : `${minutes} min` }}
            </option>
          </select>
        </label>

        <div class="flex flex-wrap gap-2">
          <button
            type="submit"
            :disabled="isBusy"
            class="text-white bg-blue rounded-full font-semibold px-5 py-2 hover:bg-darkblue disabled:opacity-50"
          >
            {{ screenLock.isEnabled ? 'Save' : 'Turn on' }}
          </button>
          <button
            v-if="screenLock.isEnabled"
            type="button"
            :disabled="isBusy"
            class="rounded-full font-semibold px-5 py-2 border border-lighter text-red-600 hover:bg-lighter disabled:opacity-50"
            @click="remove"
          >
            Turn off
          </button>
        </div>
        <p v-if="message" class="text-sm font-medium" :class="isError ? 'text-red-600' : 'text-green-700'" role="status">
          {{ message }}
        </p>
      </form>
    </div>
    <DefaultRightBar :profile="ownerProfile" />
  </div>
</template>

<script>
import {defineAsyncComponent} from "vue";
import {warpnetService} from "@/service/service";
import {
  IDLE_MINUTES_CHOICES,
  MAX_UNLOCK_ATTEMPTS,
  PIN_PATTERN,
  removeScreenLock,
  screenLock,
  updateScreenLock,
} from "@/lib/screen-lock";

export default {
  name: "SettingsScreenLock",
  components: {
    SideNav: defineAsyncComponent(() => import('@/components/SideNav.vue')),
    DefaultRightBar: defineAsyncComponent(() => import('@/components/DefaultRightBar.vue')),
  },
  data() {
    return {
      IDLE_MINUTES_CHOICES,
      MAX_UNLOCK_ATTEMPTS,
      screenLock,
      ownerProfile: warpnetService.getOwnerProfile(),
      currentPin: '',
      newPin: '',
      newPinRepeat: '',
      idleMinutes: screenLock.isEnabled ? screenLock.idleMinutes : 5,
      isBusy: false,
      message: '',
      isError: false,
    };
  },
  methods: {
    report(message, isError) {
      this.message = message;
      this.isError = isError;
    },
    async afterWrongPin() {
      if (screenLock.attemptsLeft > 0) {
        this.report(`Wrong current PIN. ${screenLock.attemptsLeft} attempts left before sign-out.`, true);
        return;
      }
      await warpnetService.logoutUser();
      this.$router.push({ name: 'Root' });
    },
    async save() {
      if (this.newPin && !PIN_PATTERN.test(this.newPin)) {
        this.report('PIN must be 4 to 8 digits.', true);
        return;
      }
      if (this.newPin && this.newPin !== this.newPinRepeat) {
        this.report('The PINs do not match.', true);
        return;
      }
      if (!screenLock.isEnabled && !this.newPin) {
        this.report('Choose a PIN first.', true);
        return;
      }
      this.isBusy = true;
      const isSaved = await updateScreenLock({
        currentPin: this.currentPin,
        newPin: this.newPin,
        idleMinutes: this.idleMinutes,
      });
      this.isBusy = false;
      this.currentPin = '';
      if (!isSaved) {
        await this.afterWrongPin();
        return;
      }
      this.newPin = '';
      this.newPinRepeat = '';
      this.report('Saved', false);
    },
    async remove() {
      this.isBusy = true;
      const isRemoved = await removeScreenLock(this.currentPin);
      this.isBusy = false;
      this.currentPin = '';
      if (!isRemoved) {
        await this.afterWrongPin();
        return;
      }
      this.idleMinutes = 5;
      this.report('Screen lock is off', false);
    },
  },
};
</script>
