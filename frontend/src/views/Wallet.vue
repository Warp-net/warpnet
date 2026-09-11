<template>
  <div id="app" class="flex container h-screen w-full">
    <SideNav />
    <div class="w-full h-full overflow-y-scroll no-scrollbar">
      <div class="px-5 py-3 border-b border-lighter flex items-center sticky top-0 bg-lightest z-10">
        <button @click="goBack()" class="rounded-full md:pr-2 focus:outline-none hover:bg-lightblue" aria-label="Back">
          <i class="fas fa-arrow-left text-blue"></i>
        </button>
        <h1 class="text-xl font-bold ml-4">Wallet</h1>
        <span class="ml-3 text-sm text-dark capitalize">{{ wallet.network }}</span>
        <button @click="refresh(true)" class="ml-auto rounded-full px-2 py-1 hover:bg-lightblue" aria-label="Refresh" :disabled="busy">
          <i class="fas fa-rotate-right text-blue" :class="busy ? 'fa-spin' : ''"></i>
        </button>
      </div>

      <div class="p-5 flex flex-col gap-5">
        <p v-if="loadError" class="text-red-700 bg-red-100 rounded-lg px-4 py-3">{{ loadError }}</p>

        <div class="bg-paper border border-lighter rounded-2xl p-5 card">
          <p class="text-dark text-sm uppercase tracking-wide">Balance</p>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 mt-1">
            <div>
              <p class="text-4xl font-bold">{{ loadingWallet ? '—' : format(wallet.usdt_balance, wallet.decimals) }} <span class="text-2xl text-dark">USDT</span></p>
              <p class="text-dark text-xs mt-1">token balance</p>
            </div>
            <div>
              <p class="text-4xl font-bold">{{ loadingWallet ? '—' : format(wallet.trx_balance, 6) }} <span class="text-2xl text-dark">TRX</span></p>
              <p class="text-dark text-xs mt-1">for network fees</p>
            </div>
          </div>
          <p v-if="!loadingWallet && wallet.address" class="text-dark text-sm mt-4 pt-3 border-t border-lighter">
            <span class="font-semibold">{{ wallet.activated ? 'Existing wallet.' : 'New wallet.' }}</span>
            {{ originText }}
          </p>
        </div>

        <div class="grid grid-cols-1 lg:grid-cols-2 gap-5">
          <div class="bg-paper border border-lighter rounded-2xl p-5 card">
            <p class="text-dark text-sm uppercase tracking-wide mb-2">Receive</p>
            <Loader :loading="loadingAddress" />
            <div v-if="!loadingAddress" class="flex flex-col items-center gap-3">
              <img v-if="qr" :src="qr" alt="Address QR" class="w-40 h-40 rounded-lg border border-lighter bg-white" />
              <div class="min-w-0 w-full text-center">
                <p class="mono break-all text-sm">{{ wallet.address }}</p>
                <button @click="copy(wallet.address, 'address')" class="mt-2 text-white bg-blue rounded-full px-4 py-1 hover:bg-darkblue">
                  {{ copied === 'address' ? 'Copied' : 'Copy address' }}
                </button>
                <p class="text-dark text-xs mt-2">Send only USDT (TRC-20) and TRX on {{ wallet.network }} to this address.</p>
              </div>
            </div>
          </div>

          <div class="bg-paper border border-lighter rounded-2xl p-5 card">
            <p class="text-dark text-sm uppercase tracking-wide mb-3">Send USDT</p>
            <div class="flex gap-4 mb-2 text-sm">
              <label class="flex items-center gap-2 cursor-pointer">
                <input type="radio" value="user" v-model="recipientMode" />
                Warpnet user
              </label>
              <label class="flex items-center gap-2 cursor-pointer">
                <input type="radio" value="address" v-model="recipientMode" />
                External address
              </label>
            </div>
            <div v-if="recipientMode === 'user'" class="mb-3">
              <select v-model="recipientUser" class="w-full text-sm px-3 py-2 rounded-lg border border-lighter bg-lightest">
                <option value="">Choose a recipient…</option>
                <option v-for="c in contacts" :key="c.user_id" :value="c.address">
                  {{ c.username || c.user_id }}
                </option>
              </select>
              <p v-if="loadingContacts" class="text-dark text-xs mt-1">Looking for the wallets of people you follow…</p>
              <p v-else-if="!contacts.length" class="text-dark text-xs mt-1">
                Nobody you follow has opened their wallet yet, so no addresses are known. Send to an external address meanwhile.
              </p>
            </div>
            <div v-else class="mb-3">
              <input v-model="sendTo" placeholder="T..." class="w-full mono text-sm px-3 py-2 rounded-lg border border-lighter bg-lightest" />
            </div>
            <label class="block text-sm text-dark mb-1">Amount (USDT)</label>
            <input v-model="sendAmount" inputmode="decimal" placeholder="0.0" class="w-full mono text-sm px-3 py-2 rounded-lg border border-lighter bg-lightest mb-3" />
            <button @click="send()" :disabled="sending || loadingAddress" class="text-white bg-blue rounded-full px-5 py-2 hover:bg-darkblue disabled:opacity-50">
              {{ sending ? 'Sending…' : 'Send' }}
            </button>
            <p v-if="sendError" class="text-red-700 mt-3">{{ sendError }}</p>
            <p v-if="sendResult" class="text-green-700 mt-3 break-all">
              Sent. Tx: <a :href="scan('tx', sendResult)" target="_blank" rel="noopener" class="mono underline">{{ sendResult }}</a>
            </p>
          </div>
        </div>

        <div class="bg-paper border border-lighter rounded-2xl p-5 card">
          <p class="text-dark text-sm uppercase tracking-wide mb-3">History</p>
          <Loader :loading="loadingHistory" />
          <p v-if="!loadingHistory && !history.length" class="text-dark">No USDT transfers yet.</p>
          <div v-for="t in history" :key="t.tx" class="flex items-center justify-between border-b border-lighter py-2 last:border-0">
            <div class="min-w-0">
              <p :class="t.incoming ? 'text-green-700' : 'text-dark'" class="font-semibold">
                {{ t.incoming ? '+' : '−' }}{{ format(t.value, wallet.decimals) }} USDT
              </p>
              <p class="text-dark text-xs mono truncate">{{ t.incoming ? ('from ' + t.from) : ('to ' + t.to) }}</p>
            </div>
            <a :href="scan('tx', t.tx)" target="_blank" rel="noopener" class="text-blue text-sm ml-3 shrink-0">view</a>
          </div>
        </div>

        <div class="bg-paper border border-lighter rounded-2xl p-5 card">
          <p class="text-dark text-sm uppercase tracking-wide mb-2">Move your wallet elsewhere</p>
          <p class="text-dark text-sm mb-3">Your private key gives full control of these funds. Never share it. Import it into TronLink or any TRON wallet to use this account there.</p>
          <button v-if="!privateKey" @click="revealKey()" :disabled="revealing" class="text-blue border border-blue rounded-full px-4 py-1 hover:bg-lightblue">
            {{ revealing ? 'Revealing…' : 'Show private key' }}
          </button>
          <div v-else>
            <p class="mono break-all text-sm bg-lightest border border-lighter rounded-lg p-3">{{ privateKey }}</p>
            <div class="flex gap-2 mt-2">
              <button @click="copy(privateKey, 'key')" class="text-white bg-blue rounded-full px-4 py-1 hover:bg-darkblue">
                {{ copied === 'key' ? 'Copied' : 'Copy private key' }}
              </button>
              <button @click="privateKey = ''" class="text-dark border border-lighter rounded-full px-4 py-1 hover:bg-lightblue">Hide</button>
            </div>
          </div>
          <p v-if="keyError" class="text-red-700 mt-3">{{ keyError }}</p>
        </div>
      </div>
    </div>
    <DefaultRightBar :profile="ownerProfile" />
  </div>
</template>

<script>
import {defineAsyncComponent} from "vue";
import {warpnetService} from "@/service/service";
import {buildQRCode} from "@/lib/qr";

const historyPollEvery = 15000;
const historyPollTries = 6;

const SCAN = {
  testnet: "https://nile.tronscan.org/#",
  mainnet: "https://tronscan.org/#",
};

export default {
  name: "Wallet",
  components: {
    SideNav: defineAsyncComponent(() => import('@/components/SideNav.vue')),
    DefaultRightBar: defineAsyncComponent(() => import('@/components/DefaultRightBar.vue')),
    Loader: defineAsyncComponent(() => import('@/components/Loader.vue')),
  },
  data() {
    return {
      loadingAddress: true,
      loadingWallet: true,
      loadingHistory: true,
      loadingContacts: true,
      loadError: "",
      ownerProfile: {},
      wallet: {address: "", usdt_balance: "0", trx_balance: "0", decimals: 6, network: "", token: ""},
      qr: "",
      history: [],
      contacts: [],
      contactsTimer: null,
      historyTimer: null,
      recipientMode: "user",
      recipientUser: "",
      sendTo: "",
      sendAmount: "",
      sending: false,
      sendError: "",
      sendResult: "",
      revealing: false,
      privateKey: "",
      keyError: "",
      copied: "",
    };
  },
  computed: {
    busy() {
      return this.loadingAddress || this.loadingWallet || this.loadingHistory || this.loadingContacts;
    },
    originText() {
      const net = this.wallet.network || "this network";
      if (!this.wallet.activated) {
        return "Warpnet created it automatically from your account: your login and password derive the key, "
          + "so signing in again always restores the very same address. It has no history on " + net
          + " yet and becomes active on the first incoming transfer.";
      }
      const when = this.wallet.created_at
        ? new Date(this.wallet.created_at).toLocaleDateString()
        : "";
      return "This address already exists on " + net + (when ? ", active since " + when : "")
        + ". Warpnet re-derived it from your account rather than creating a new one, so the funds on it are yours.";
    },
  },
  methods: {
    goBack() { this.$router.back(); },
    scan(kind, value) {
      const base = SCAN[this.wallet.network] || SCAN.testnet;
      return `${base}/${kind === 'tx' ? 'transaction' : 'address'}/${value}`;
    },
    format(units, decimals) {
      try {
        const v = BigInt(units || "0");
        const base = 10n ** BigInt(decimals);
        const whole = (v / base).toString();
        const frac = (v % base).toString().padStart(decimals, "0").replace(/0+$/, "");
        return frac ? `${whole}.${frac}` : whole;
      } catch { return "0"; }
    },
    parse(amount, decimals) {
      const s = String(amount).trim();
      if (!/^\d+(\.\d+)?$/.test(s)) throw new Error("Enter a number like 12.5");
      const [w, f = ""] = s.split(".");
      if (f.length > decimals) throw new Error(`At most ${decimals} decimals`);
      const units = BigInt(w) * 10n ** BigInt(decimals) + BigInt((f.padEnd(decimals, "0")) || "0");
      if (units <= 0n) throw new Error("Amount must be positive");
      return units.toString();
    },
    async loadAddress() {
      this.loadingAddress = true;
      try {
        const a = await warpnetService.getWalletAddress();
        if (a && a.address) {
          this.wallet = {...this.wallet, ...a};
        }
      } catch {
        this.wallet.address = this.wallet.address || "";
      } finally {
        this.loadingAddress = false;
      }
      await this.ensureQR();
    },
    async loadWallet() {
      this.loadingWallet = true;
      this.loadError = "";
      try {
        const w = await warpnetService.getWallet();
        if (w && w.address) {
          this.wallet = {...this.wallet, ...w};
        }
      } catch (err) {
        this.loadError = (err && err.message) || "Failed to load wallet";
      } finally {
        this.loadingWallet = false;
      }
      await this.ensureQR();
    },
    async ensureQR() {
      if (this.qr || !this.wallet.address) return;
      this.qr = await buildQRCode(this.wallet.address).catch(() => "");
    },
    async loadHistory(quiet) {
      if (!quiet) this.loadingHistory = true;
      try {
        this.history = await warpnetService.getWalletHistory(25);
      } catch {
        if (!quiet) this.history = [];
      } finally {
        this.loadingHistory = false;
      }
    },
    watchForTransfer(tx) {
      clearTimeout(this.historyTimer);
      if (!tx) return;
      let attempts = 0;
      const poll = async () => {
        attempts++;
        await this.loadHistory(true);
        if (this.history.some((t) => t.tx === tx) || attempts >= historyPollTries) return;
        this.historyTimer = setTimeout(poll, historyPollEvery);
      };
      this.historyTimer = setTimeout(poll, historyPollEvery);
    },
    async loadContacts(force) {
      this.loadingContacts = true;
      try {
        this.contacts = await warpnetService.getWalletContacts(force);
      } catch {
        this.contacts = [];
      } finally {
        this.loadingContacts = false;
      }
    },
    scheduleContactsRefresh() {
      clearTimeout(this.contactsTimer);
      this.contactsTimer = setTimeout(() => this.loadContacts(), 6000);
    },
    refresh(force) {
      this.loadAddress();
      this.loadWallet();
      this.loadHistory();
      this.loadContacts(force).then(() => this.scheduleContactsRefresh());
    },
    async send() {
      this.sendError = "";
      this.sendResult = "";
      let amount;
      try {
        amount = this.parse(this.sendAmount, this.wallet.decimals);
      } catch (err) {
        this.sendError = err.message;
        return;
      }
      const to = this.recipientMode === "user" ? this.recipientUser : this.sendTo.trim();
      if (!to) {
        this.sendError = this.recipientMode === "user" ? "Choose a recipient" : "Enter a recipient address";
        return;
      }
      this.sending = true;
      try {
        const resp = await warpnetService.sendUsdt(to, amount);
        this.sendResult = resp && resp.tx;
        this.sendTo = "";
        this.recipientUser = "";
        this.sendAmount = "";
        this.loadWallet();
        this.loadHistory();
        this.watchForTransfer(this.sendResult);
      } catch (err) {
        this.sendError = (err && err.message) || "Transfer failed";
      } finally {
        this.sending = false;
      }
    },
    async revealKey() {
      this.keyError = "";
      this.revealing = true;
      try {
        const resp = await warpnetService.exportWalletKey();
        this.privateKey = resp && resp.private_key;
      } catch (err) {
        this.keyError = (err && err.message) || "Could not reveal the key";
      } finally {
        this.revealing = false;
      }
    },
    async copy(text, what) {
      try {
        await navigator.clipboard.writeText(text);
        this.copied = what;
        setTimeout(() => { this.copied = ""; }, 1500);
      } catch { /* clipboard unavailable */ }
    },
  },
  created() {
    this.ownerProfile = warpnetService.getOwnerProfile();
    this.refresh();
  },
  beforeUnmount() {
    clearTimeout(this.contactsTimer);
    clearTimeout(this.historyTimer);
  },
};
</script>
