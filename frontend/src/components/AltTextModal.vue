<!-- Warpnet - Decentralized Social Network -->
<template>
  <div
    v-if="show"
    class="fixed inset-0 bg-black bg-opacity-50 z-50 flex items-center justify-center p-4"
    @click.self.stop="$emit('close')"
    @click.stop
  >
    <div class="bg-white rounded-lg w-full max-w-md flex flex-col">
      <div class="px-5 py-3 border-b border-lighter flex items-center">
        <h2 class="font-bold text-lg">Describe image</h2>
        <button
          @click="$emit('close')"
          class="ml-auto text-dark hover:text-black"
          aria-label="Close"
        >
          <i class="fas fa-times"></i>
        </button>
      </div>
      <div class="p-5">
        <img v-if="previewUrl" :src="previewUrl" alt="" class="w-full max-h-64 object-contain rounded border border-lighter mb-3" />
        <label class="block mb-2">
          <span class="text-sm font-bold">Alt text</span>
          <textarea
            v-model="description"
            rows="3"
            placeholder="Describe this image for people who can't see it"
            maxlength="1500"
            class="mt-1 w-full rounded border border-lighter bg-white p-2 text-sm"
          ></textarea>
          <span class="text-xs text-dark">{{ description.length }} / 1500</span>
        </label>
        <div class="flex justify-end gap-2 mt-3">
          <button
            @click="$emit('close')"
            class="px-3 py-1 rounded-full border border-lighter hover:bg-lighter"
          >Cancel</button>
          <button
            @click="save"
            :disabled="saving"
            class="text-white bg-blue rounded-full font-semibold px-4 py-1 hover:bg-darkblue disabled:opacity-50"
          >{{ saving ? 'Saving…' : 'Save' }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import {warpnetService} from "@/service/service";

export default {
  name: "AltTextModal",
  props: {
    show: { type: Boolean, default: false },
    imageKey: { type: String, required: true },
    previewUrl: { type: String, default: '' },
  },
  emits: ['close', 'saved'],
  data() {
    return { description: '', saving: false };
  },
  watch: {
    show: {
      immediate: true,
      async handler(v) {
        if (!v) return;
        try {
          const meta = await warpnetService.getMediaMeta(this.imageKey);
          this.description = meta?.description || '';
        } catch (err) {
          console.error('Failed to load media meta:', err);
        }
      },
    },
  },
  methods: {
    async save() {
      this.saving = true;
      try {
        await warpnetService.updateMediaMeta(this.imageKey, this.description, 0, 0);
        this.$emit('saved', { key: this.imageKey, description: this.description });
        this.$emit('close');
      } catch (err) {
        console.error('Failed to save media meta:', err);
      } finally {
        this.saving = false;
      }
    },
  },
};
</script>
