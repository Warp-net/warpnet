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
            </div>
            <div>
              <p class="text-4xl font-bold">{{ loadingWallet ? '—' : format(wallet.trx_balance, 6) }} <span class="text-2xl text-dark">TRX</span></p>
            </div>
          </div>
          <p v-if="!loadingWallet && wallet.address" class="text-dark text-sm mt-4 pt-3 border-t border-lighter">
            <span class="font-semibold">{{ originLead }}</span>
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
                <p v-if="copyFailed === 'address'" class="text-red-700 text-xs mt-2">
                  This page cannot reach the clipboard — it is served over plain HTTP, which browsers do not trust with it.
                  Select the address above and copy it by hand.
                </p>
                <p class="text-dark text-xs mt-2">Send only USDT (TRC-20) and TRX on {{ wallet.network }} to this address.</p>
              </div>
            </div>
          </div>

          <div class="bg-paper border border-lighter rounded-2xl p-5 card">
            <p class="text-dark text-sm uppercase tracking-wide mb-3">Send</p>
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
            <div v-if="recipientMode === 'user'" class="mb-3" ref="recipientMenu">
              <button
                type="button"
                role="combobox"
                aria-haspopup="listbox"
                :aria-expanded="recipientOpen"
                :disabled="!contacts.length"
                @click="recipientOpen = !recipientOpen"
                class="w-full text-sm px-3 py-2 rounded-lg border border-lighter bg-lightest flex items-center gap-2 text-left disabled:opacity-60"
              >
                <template v-if="selectedContact">
                  <img v-if="selectedContact.avatar" :src="selectedContact.avatar" alt="" class="w-8 h-8 rounded-full object-cover shrink-0" />
                  <span v-else class="w-8 h-8 rounded-full bg-lighter shrink-0 flex items-center justify-center text-xs">{{ initials(selectedContact) }}</span>
                  <span class="min-w-0 flex-1">
                    <span class="block truncate">{{ selectedContact.username || selectedContact.user_id }}</span>
                    <span class="block text-dark text-xs mono truncate">{{ selectedContact.user_id }}</span>
                  </span>
                </template>
                <span v-else class="flex-1">Choose a recipient…</span>
                <i class="fas fa-chevron-down text-xs text-dark shrink-0" aria-hidden="true"></i>
              </button>
              <ul v-if="recipientOpen" role="listbox" class="mt-1 max-h-56 overflow-y-auto rounded-lg border border-lighter bg-lightest">
                <li v-for="c in contacts" :key="c.user_id">
                  <button
                    type="button"
                    role="option"
                    :aria-selected="c.address === recipientUser"
                    @click="pickRecipient(c)"
                    class="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-lightblue"
                  >
                    <img v-if="c.avatar" :src="c.avatar" alt="" class="w-8 h-8 rounded-full object-cover shrink-0" />
                    <span v-else class="w-8 h-8 rounded-full bg-lighter shrink-0 flex items-center justify-center text-xs">{{ initials(c) }}</span>
                    <span class="min-w-0">
                      <span class="block truncate text-sm">{{ c.username || c.user_id }}</span>
                      <span class="block text-dark text-xs mono truncate">{{ c.user_id }}</span>
                    </span>
                  </button>
                </li>
              </ul>
              <p v-if="loadingContacts" class="text-dark text-xs mt-1">Looking for the wallets of people you follow…</p>
              <p v-else-if="!contacts.length" class="text-dark text-xs mt-1">
                Nobody you follow has opened their wallet yet, so no addresses are known. Send to an external address meanwhile.
              </p>
            </div>
            <div v-else class="mb-3">
              <input v-model="sendTo" placeholder="T..." class="w-full mono text-sm px-3 py-2 rounded-lg border border-lighter bg-lightest" />
            </div>
            <div class="flex items-center justify-between mb-1">
              <label class="block text-sm text-dark">Amount</label>
              <div class="flex gap-1" role="group" aria-label="Asset to send">
                <button
                  v-for="a in assets"
                  :key="a"
                  type="button"
                  :aria-pressed="asset === a"
                  @click="sendAsset = a"
                  class="rounded-full px-3 py-0.5 text-xs"
                  :class="asset === a ? 'bg-blue text-white' : 'text-dark border border-lighter hover:bg-lightblue'"
                >{{ a }}</button>
              </div>
            </div>
            <p class="text-dark text-xs mb-1">You hold {{ format(sendable, assetDecimals(asset)) }} {{ asset }}.</p>
            <input v-model="sendAmount" inputmode="decimal" placeholder="0.0" class="w-full mono text-sm px-3 py-2 rounded-lg border border-lighter bg-lightest mb-3" />
            <button @click="send()" :disabled="sending || loadingWallet || !hasTrx" class="text-white bg-blue rounded-full px-5 py-2 hover:bg-darkblue disabled:opacity-50">
              {{ sending ? 'Sending…' : 'Send' }}
            </button>
            <p v-if="!loadingWallet && !hasTrx" class="text-red-700 text-sm mt-3">
              This address holds no TRX. A USDT transfer pays its network fee in TRX, so the network would refuse it.
              Send some TRX here first.
            </p>
            <p v-if="sendError" class="text-red-700 mt-3">{{ sendError }}</p>
            <p v-if="sendResult" class="text-green-700 mt-3 break-all">
              Sent. Tx: <a :href="scan('tx', sendResult)" target="_blank" rel="noopener" class="mono underline">{{ sendResult }}</a>
            </p>
          </div>
        </div>

        <div class="bg-paper border border-lighter rounded-2xl p-5 card">
          <p class="text-dark text-sm uppercase tracking-wide mb-3">History</p>
          <Loader :loading="loadingHistory" />
          <p v-if="historyError" class="text-red-700">{{ historyError }}</p>
          <p v-else-if="!loadingHistory && !history.length" class="text-dark">No transfers yet.</p>
          <div v-for="t in history" :key="t.tx" class="flex items-center justify-between border-b border-lighter py-2 last:border-0">
            <div class="min-w-0">
              <p :class="t.incoming ? 'text-green-700' : 'text-dark'" class="font-semibold">
                {{ t.incoming ? '+' : '−' }}{{ format(t.value, assetDecimals(t.asset)) }} {{ t.asset || wallet.token }}
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
            <p v-if="copyFailed === 'key'" class="text-red-700 text-xs mt-2">
              This page cannot reach the clipboard — it is served over plain HTTP, which browsers do not trust with it.
              Select the key above and copy it by hand.
            </p>
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
const historyRows = 25;
const nativeCoin = "TRX";
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
      historyError: "",
      sendAsset: "",
      recipientMode: "user",
      recipientUser: "",
      recipientOpen: false,
      sendTo: "",
      sendAmount: "",
      sending: false,
      sendError: "",
      sendResult: "",
      revealing: false,
      privateKey: "",
      keyError: "",
      copied: "",
      copyFailed: "",
    };
  },
  computed: {
    selectedContact() {
      return this.contacts.find((c) => c.address === this.recipientUser) || null;
    },
    busy() {
      return this.loadingAddress || this.loadingWallet || this.loadingHistory || this.loadingContacts;
    },
    hasTrx() {
      return this.units(this.wallet.trx_balance) > 0n;
    },
    assets() {
      return [this.wallet.token || "USDT", nativeCoin];
    },
    asset() {
      return this.sendAsset || this.assets[0];
    },
    sendable() {
      return this.asset === nativeCoin ? this.wallet.trx_balance : this.wallet.usdt_balance;
    },
    inUse() {
      return this.units(this.wallet.usdt_balance) > 0n || this.hasTrx || this.history.length > 0;
    },
    originLead() {
      if (this.wallet.activated) return "Existing wallet.";
      return this.inUse ? "Not activated yet." : "New wallet.";
    },
    originText() {
      const net = this.wallet.network || "this network";
      const derived = "Warpnet created it automatically from your account: your login and password derive the key, "
        + "so signing in again always restores the very same address.";
      if (this.wallet.activated) {
        const when = this.wallet.created_at
          ? new Date(this.wallet.created_at).toLocaleDateString()
          : "";
        return "This address already exists on " + net + (when ? ", active since " + when : "")
          + ". Warpnet re-derived it from your account rather than creating a new one, so the funds on it are yours.";
      }
      if (this.inUse) {
        return derived + " It already holds funds, but TRON has not activated the account behind it: on " + net
          + " an address is activated by incoming TRX, and receiving USDT does not do it. Until some TRX arrives,"
          + " this address cannot pay a network fee, so it can receive but not send.";
      }
      return derived + " It becomes active once it receives TRX on " + net
        + ", which is what pays the network fee on anything it sends.";
    },
  },
  methods: {
    goBack() { this.$router.back(); },
    scan(kind, value) {
      const base = SCAN[this.wallet.network] || SCAN.testnet;
      return `${base}/${kind === 'tx' ? 'transaction' : 'address'}/${value}`;
    },
    units(value) {
      try {
        return BigInt(value || "0");
      } catch {
        return 0n;
      }
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
      const asked = await Promise.allSettled(this.assets.map((a) => warpnetService.getWalletHistory(25, a)));
      this.loadingHistory = false;
      const answered = asked.filter((r) => r.status === "fulfilled");
      if (!answered.length) {
        if (!quiet) this.historyError = "Could not read the transfer history.";
        return;
      }
      this.historyError = "";
      this.history = answered
        .flatMap((r) => r.value || [])
        .sort((a, b) => (b.timestamp || 0) - (a.timestamp || 0))
        .slice(0, historyRows);
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
      await this.loadAvatars();
    },
    async loadAvatars() {
      await Promise.all(this.contacts.map(async (c) => {
        if (!c || !c.avatar_key || c.avatar) return;
        c.avatar = await warpnetService.getImage({userId: c.user_id, key: c.avatar_key}).catch(() => null);
      }));
    },
    pickRecipient(contact) {
      this.recipientUser = contact.address;
      this.recipientOpen = false;
    },
    assetDecimals(asset) {
      return asset === nativeCoin ? 6 : this.wallet.decimals;
    },
    initials(contact) {
      const name = (contact && (contact.username || contact.user_id)) || "?";
      return name.trim().charAt(0).toUpperCase();
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
        amount = this.parse(this.sendAmount, this.assetDecimals(this.asset));
      } catch (err) {
        this.sendError = err.message;
        return;
      }
      if (this.units(amount) > this.units(this.sendable)) {
        this.sendError = `That is more than this address holds in ${this.asset}.`;
        return;
      }
      const to = this.recipientMode === "user" ? this.recipientUser : this.sendTo.trim();
      if (!to) {
        this.sendError = this.recipientMode === "user" ? "Choose a recipient" : "Enter a recipient address";
        return;
      }
      this.sending = true;
      try {
        const resp = await warpnetService.sendFunds(to, amount, this.asset);
        this.sendResult = resp && resp.tx;
        this.sendTo = "";
        this.recipientUser = "";
        this.sendAmount = "";
        this.loadWallet();
        this.loadHistory();
        this.watchForTransfer(this.sendResult);
      } catch (err) {
        this.sendError = (err && (err.message || err.code)) || "Transfer failed";
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
      if (!text) return;
      this.copyFailed = "";
      if (await this.writeClipboard(text)) {
        this.copied = what;
        setTimeout(() => { this.copied = ""; }, 1500);
        return;
      }
      this.copyFailed = what;
    },
    // The Clipboard API only exists in a secure context, and a node served over
    // plain HTTP is not one, so fall back to the legacy selection copy.
    async writeClipboard(text) {
      try {
        if (navigator.clipboard && navigator.clipboard.writeText) {
          await navigator.clipboard.writeText(text);
          return true;
        }
      } catch { /* the legacy path below still has a chance */ }
      try {
        const area = document.createElement("textarea");
        area.value = text;
        area.setAttribute("readonly", "");
        area.style.position = "fixed";
        area.style.top = "0";
        area.style.opacity = "0";
        document.body.appendChild(area);
        area.select();
        const copied = document.execCommand && document.execCommand("copy");
        document.body.removeChild(area);
        return Boolean(copied);
      } catch {
        return false;
      }
    },
  },
  created() {
    this.ownerProfile = warpnetService.getOwnerProfile();
    this.refresh();
  },
  mounted() {
    this.onDocClick = (e) => {
      const menu = this.$refs.recipientMenu;
      if (this.recipientOpen && menu && !menu.contains(e.target)) {
        this.recipientOpen = false;
      }
    };
    this.onDocKeyup = (e) => {
      if (e.key === "Escape") this.recipientOpen = false;
    };
    document.addEventListener("click", this.onDocClick);
    window.addEventListener("keyup", this.onDocKeyup);
  },
  beforeUnmount() {
    clearTimeout(this.contactsTimer);
    clearTimeout(this.historyTimer);
    document.removeEventListener("click", this.onDocClick);
    window.removeEventListener("keyup", this.onDocKeyup);
  },
};
</script>
