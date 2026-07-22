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
        <h1 class="text-xl font-bold ml-4">Filters</h1>
      </div>

      <Loader :loading="loading" />

      <form @submit.prevent="addFilter" class="p-5 border-b border-lighter flex gap-2">
        <input
          v-model="newTitle"
          type="text"
          placeholder="New filter name"
          required
          class="flex-1 rounded border border-lighter bg-white p-2"
        />
        <button
          type="submit"
          class="text-white bg-blue rounded-full font-semibold px-4 py-2 hover:bg-darkblue"
        >
          Add
        </button>
      </form>

      <div
        v-if="!loading && filters.length === 0"
        class="flex flex-col items-center justify-center pt-10 px-5"
      >
        <p class="font-bold text-lg">No filters yet</p>
        <p class="text-sm text-dark">Create a filter to hide tweets containing certain keywords.</p>
      </div>

      <div v-for="f in filters" :key="f.id" class="border-b border-lighter">
        <div class="px-5 py-3 flex items-center gap-2">
          <input
            v-model="f.title"
            type="text"
            class="font-bold bg-transparent flex-1 focus:outline-none focus:bg-white focus:rounded focus:px-1 focus:border focus:border-lighter"
            @change="renameFilter(f)"
          />
          <button
            @click="deleteFilter(f.id)"
            class="text-red-600 hover:bg-red-50 rounded-full px-3 py-1"
          >
            Delete
          </button>
        </div>
        <div class="px-5 pb-3">
          <div v-for="kw in f.keywords || []" :key="kw.id" class="flex items-center text-sm py-1">
            <input
              v-model="kw.keyword"
              type="text"
              class="bg-lighter rounded-full px-2 py-0.5 focus:outline-none focus:bg-white"
              @change="renameKeyword(f.id, kw)"
            />
            <button
              @click="removeKeyword(f.id, kw.id)"
              class="ml-2 text-dark hover:text-red-600"
              aria-label="Remove keyword"
            >
              <i class="fas fa-times text-xs"></i>
            </button>
          </div>
          <form @submit.prevent="addKeyword(f)" class="flex gap-2 mt-2">
            <input
              v-model="keywordDrafts[f.id]"
              type="text"
              placeholder="Add keyword"
              class="flex-1 rounded border border-lighter bg-white p-1 text-sm"
            />
            <button
              type="submit"
              class="text-blue text-sm hover:underline"
            >
              Add keyword
            </button>
          </form>
        </div>
      </div>
    </div>
    <DefaultRightBar :profile="ownerProfile" />
  </div>
</template>

<script>
import {defineAsyncComponent} from "vue";
import {warpnetService} from "@/service/service";
import {toast} from "@/lib/toast";

export default {
  name: "SettingsFilters",
  components: {
    SideNav: defineAsyncComponent(() => import('@/components/SideNav.vue')),
    DefaultRightBar: defineAsyncComponent(() => import('@/components/DefaultRightBar.vue')),
    Loader: defineAsyncComponent(() => import('@/components/Loader.vue')),
  },
  data() {
    return {
      loading: true,
      filters: [],
      newTitle: '',
      keywordDrafts: {},
      ownerProfile: {},
    };
  },
  methods: {
    async reload() {
      const resp = await warpnetService.getFilters();
      this.filters = (resp?.filters || resp || []).map(f => ({
        ...f,
        keywords: f.keywords || [],
      }));
    },
    async addFilter() {
      if (!this.newTitle.trim()) return;
      try {
        await warpnetService.createFilter({ title: this.newTitle.trim(), context: ['home'], action: 'hide' });
        this.newTitle = '';
        await this.reload();
      } catch (err) {
        console.error('Failed to create filter', err);
        toast.error(err?.message || "Couldn't create the filter. Please try again.");
      }
    },
    async deleteFilter(id) {
      try {
        await warpnetService.deleteFilter(id);
        this.filters = this.filters.filter(f => f.id !== id);
      } catch (err) {
        console.error('Failed to delete filter', err);
        toast.error(err?.message || "Couldn't delete the filter. Please try again.");
      }
    },
    async addKeyword(f) {
      const word = (this.keywordDrafts[f.id] || '').trim();
      if (!word) return;
      try {
        await warpnetService.addFilterKeyword(f.id, word);
        this.keywordDrafts[f.id] = '';
        await this.reload();
      } catch (err) {
        console.error('Failed to add keyword', err);
        toast.error(err?.message || "Couldn't add the keyword. Please try again.");
      }
    },
    async removeKeyword(filterId, keywordId) {
      try {
        await warpnetService.deleteFilterKeyword(keywordId);
        await this.reload();
      } catch (err) {
        console.error('Failed to remove keyword', err);
        toast.error(err?.message || "Couldn't remove the keyword. Please try again.");
      }
    },
    async renameFilter(f) {
      if (!f.title || !f.title.trim()) return;
      try {
        await warpnetService.updateFilter({ id: f.id, title: f.title.trim(), context: f.context, action: f.action });
        toast.success('Filter saved.');
      } catch (err) {
        console.error('Failed to rename filter', err);
        toast.error(err?.message || "Couldn't save the filter. Please try again.");
      }
    },
    async renameKeyword(filterId, kw) {
      if (!kw.keyword || !kw.keyword.trim()) return;
      try {
        await warpnetService.updateFilterKeyword(kw.id, kw.keyword.trim(), kw.whole_word || false);
        toast.success('Keyword saved.');
      } catch (err) {
        console.error('Failed to rename keyword', err);
        toast.error(err?.message || "Couldn't save the keyword. Please try again.");
      }
    },
  },
  async created() {
    this.ownerProfile = warpnetService.getOwnerProfile();
    try {
      await this.reload();
    } catch (err) {
      console.error('Failed to load filters:', err);
    } finally {
      this.loading = false;
    }
  },
};
</script>
