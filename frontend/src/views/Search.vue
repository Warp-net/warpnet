<!-- Warpnet - Decentralized Social Network -->
<template>
  <div id="app" class="flex container h-screen w-full">
    <div class="flex container h-screen w-full">
      <SideNav />

      <div
        class="w-full md:w-1/2 h-full overflow-y-scroll no-scrollbar"
        v-scroll:bottom="loadMore"
      >
        <div class="px-5 py-3 border-lighter flex items-center">
          <button
            @click="gotoHome()"
            class="rounded-full md:pr-2 focus:outline-none hover:bg-lightblue"
          >
            <i class="fas fa-arrow-left text-blue"></i>
          </button>
          <div class="lg:block ml-4 w-full">
            <i class="fas fa-search absolute mt-3 ml-5 text-m text-light"></i>
            <input
              class="pl-12 rounded-full w-full p-2 bg-lighter text-m focus:bg-white focus:outline-none focus:ring-2 focus:ring-blue"
              placeholder="Search Warpnet"
              type="search"
              v-model="query"
              v-on:keyup.enter="submit()"
            />
          </div>
        </div>

        <div class="flex flex-col">
          <div class="flex flex-row justify-evenly">
            <button
              @click="submit('People')"
              class="w-full text-dark font-bold border-b-2 p-1 md:px-5 md:py-4 hover:bg-lightblue"
              :class="`${this.mode === 'People' ? 'border-blue' : ''}`"
            >
              People
            </button>
          </div>
        </div>

        <Loader :loading="loading" />

        <div
          v-if="!loading && results.length === 0 && submitted"
          class="flex flex-col items-center justify-center w-full pt-10"
        >
          <div class="w-3/5">
            <p class="font-bold text-lg">No results for "{{ noResults }}"</p>
            <p class="text-sm text-dark">
              Try a different search term.
            </p>
          </div>
        </div>

        <Users :users="results" :loading="loading" />
      </div>

      <div
        class="hidden md:block w-1/3 z-0 h-full border-l border-lighter px-6 py-2 overflow-y-scroll no-scrollbar relative"
      ></div>
    </div>
  </div>
</template>

<script>
import SideNav from "../components/SideNav.vue";
import Users from "../components/Users.vue";
import Loader from "../components/Loader.vue";
import {warpnetService} from "@/service/service";

export default {
  name: "Search",
  components: {
    SideNav,
    Users,
    Loader,
  },
  data() {
    return {
      loading: false,
      submitted: false,
      query: this.$route.query.q || "",
      noResults: "",
      mode: this.$route.query.m || "People",
      results: [],
      cursor: '',
    };
  },
  methods: {
    gotoHome() {
      this.$router.push({
        name: "Home",
      });
    },
    async submit(m = this.mode) {
      this.mode = m;
      this.submitted = true;
      this.cursor = '';
      this.results = [];
      this.noResults = this.query;
      await this.runSearch(true);
    },
    async runSearch(reset) {
      if (!this.query) {
        this.results = [];
        return;
      }
      this.loading = true;
      try {
        const resp = await warpnetService.searchUsers(this.query, reset ? '' : this.cursor);
        const users = resp?.users || [];
        const hydrated = await Promise.all(users.map(async (u) => {
          try {
            if (u.avatar_key && !u.avatar) {
              u.avatar = await warpnetService.getImage({userId: u.id, key: u.avatar_key});
            }
          } catch (e) {}
          return u;
        }));
        this.results = reset ? hydrated : this.results.concat(hydrated);
        this.cursor = resp?.cursor || 'end';
      } catch (err) {
        console.error('Search failed:', err);
      } finally {
        this.loading = false;
      }
    },
    async loadMore() {
      if (this.loading || !this.cursor || this.cursor === 'end') return;
      await this.runSearch(false);
    },
  },
  async created() {
    console.log("loading component:", this.$options.name);
    if (this.query) await this.submit();
  },
};
</script>
