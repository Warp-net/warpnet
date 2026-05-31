<!-- Warpnet - Decentralized Social Network -->
<template>
  <div
    v-if="show"
    class="fixed inset-0 bg-black bg-opacity-50 z-50 flex items-center justify-center p-4"
    @click.self.stop="onClose"
    @click.stop
  >
    <div class="bg-white rounded-lg w-full max-w-lg flex flex-col">
      <div class="px-5 py-3 border-b border-lighter flex items-center">
        <h2 class="font-bold text-lg">Import tweets from X</h2>
        <button
          @click="onClose"
          class="ml-auto text-dark hover:text-black"
          aria-label="Close"
        >
          <i class="fas fa-times"></i>
        </button>
      </div>
      <div class="p-5">
        <input
          ref="fileInput"
          type="file"
          accept=".zip,application/zip"
          class="hidden"
          @change="onFileChange"
        />
        <!-- instructions -->
        <template v-if="phase === 'idle'">
          <p class="text-sm text-dark mb-3">
            Import your original tweets (with attached photos) from your X
            (Twitter) data archive. Replies, retweets, likes, direct messages,
            GIFs, videos and profile data are skipped.
          </p>
          <p class="text-sm font-bold mb-1">How to get your archive from x.com:</p>
          <ol class="list-decimal list-inside text-sm text-dark space-y-1 mb-4">
            <li>On x.com open <span class="font-semibold">Settings → Your account → Download an archive of your data</span>.</li>
            <li>Confirm your password and request the archive.</li>
            <li>X prepares a <span class="font-semibold">.zip</span> file and emails you when it's ready (this can take a day or more).</li>
            <li>Download the <span class="font-semibold">.zip</span> archive to this computer.</li>
            <li>Click the button below and select that <span class="font-semibold">.zip</span> file.</li>
          </ol>
          <div class="flex justify-end gap-2">
            <button
              @click="onClose"
              class="px-3 py-1 rounded-full border border-lighter hover:bg-lighter"
            >Cancel</button>
            <button
              @click="chooseAndImport"
              class="text-white bg-blue rounded-full font-semibold px-4 py-1 hover:bg-darkblue"
            >Choose archive…</button>
          </div>
        </template>

        <!-- importing -->
        <template v-else-if="phase === 'importing'">
          <div class="flex flex-col items-center justify-center py-8">
            <i class="fas fa-spinner fa-spin text-3xl text-blue mb-3"></i>
            <p class="font-semibold">Importing your tweets…</p>
            <p class="text-sm text-dark text-center mt-1">
              This can take a few minutes for a large archive. Please keep this
              window open.
            </p>
          </div>
        </template>

        <!-- done -->
        <template v-else-if="phase === 'done'">
          <div class="flex flex-col items-center py-6">
            <i class="fas fa-check-circle text-3xl text-blue mb-3"></i>
            <p class="font-semibold mb-2">Import complete</p>
            <ul class="text-sm text-dark space-y-1">
              <li><span class="font-semibold">{{ result.imported_tweets }}</span> tweets imported</li>
              <li><span class="font-semibold">{{ result.imported_images }}</span> images imported</li>
              <li><span class="font-semibold">{{ result.skipped_tweets }}</span> skipped (replies, retweets, already imported)</li>
            </ul>
            <p class="text-sm text-dark mt-3 text-center">
              Your imported tweets are now on your profile.
            </p>
          </div>
          <div class="flex justify-end">
            <button
              @click="onClose"
              class="text-white bg-blue rounded-full font-semibold px-4 py-1 hover:bg-darkblue"
            >Done</button>
          </div>
        </template>

        <!-- error -->
        <template v-else>
          <div class="flex flex-col items-center py-6">
            <i class="fas fa-exclamation-circle text-3xl text-red-600 mb-3"></i>
            <p class="font-semibold mb-2">Import failed</p>
            <p class="text-sm text-dark text-center break-words">{{ errorMessage }}</p>
          </div>
          <div class="flex justify-end gap-2">
            <button
              @click="onClose"
              class="px-3 py-1 rounded-full border border-lighter hover:bg-lighter"
            >Close</button>
            <button
              @click="phase = 'idle'"
              class="text-white bg-blue rounded-full font-semibold px-4 py-1 hover:bg-darkblue"
            >Try again</button>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>

<script>
import {warpnetService} from "@/service/service";

export default {
  name: "ImportTweetsModal",
  props: {
    show: { type: Boolean, default: false },
  },
  emits: ['close', 'imported'],
  data() {
    return {
      phase: 'idle', // idle | importing | done | error
      result: { imported_tweets: 0, imported_images: 0, skipped_tweets: 0 },
      errorMessage: '',
    };
  },
  watch: {
    show(v) {
      if (v) {
        // Reset to a clean state every time the modal is reopened.
        this.phase = 'idle';
        this.errorMessage = '';
      }
    },
  },
  methods: {
    onClose() {
      if (this.phase === 'importing') return; // don't close mid-import
      this.$emit('close');
    },
    async chooseAndImport() {
      // Browser dashboard (business node): no native dialog — pick a file and
      // upload its bytes. Desktop (Wails member node): native dialog returns a
      // path the node reads straight off local disk.
      if (!warpnetService.isDesktopNode()) {
        this.$refs.fileInput.click();
        return;
      }
      let path = '';
      try {
        path = await warpnetService.openTwitterArchiveDialog();
      } catch (err) {
        console.error('Failed to open archive dialog:', err);
        this.errorMessage = 'Could not open the file picker.';
        this.phase = 'error';
        return;
      }
      if (!path) {
        return; // user cancelled the picker
      }
      await this.runImport({ archivePath: path });
    },
    async onFileChange(e) {
      const file = e.target.files && e.target.files[0];
      e.target.value = ''; // allow re-selecting the same file later
      if (!file) return;
      this.phase = 'importing';
      try {
        const archiveData = await this.readFileAsDataURL(file);
        await this.runImport({ archiveData });
      } catch (err) {
        console.error('Failed to read archive:', err);
        this.errorMessage = 'Could not read the selected file.';
        this.phase = 'error';
      }
    },
    readFileAsDataURL(file) {
      return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(reader.result); // data:...;base64,XXXX
        reader.onerror = () => reject(new Error('file read error'));
        reader.readAsDataURL(file);
      });
    },
    async runImport(payload) {
      this.phase = 'importing';
      try {
        const resp = await warpnetService.importTwitterArchive(payload);
        if (resp && resp.code) {
          throw new Error(resp.message || 'Import failed');
        }
        this.result = {
          imported_tweets: resp?.imported_tweets || 0,
          imported_images: resp?.imported_images || 0,
          skipped_tweets: resp?.skipped_tweets || 0,
        };
        this.phase = 'done';
        this.$emit('imported', this.result);
      } catch (err) {
        console.error('Tweet import failed:', err);
        this.errorMessage = (err && err.message) ? err.message : 'Import failed. Please try again.';
        this.phase = 'error';
      }
    },
  },
};
</script>
