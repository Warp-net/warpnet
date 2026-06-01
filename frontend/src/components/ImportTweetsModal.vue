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
            <p v-if="progressText" class="text-sm font-semibold text-blue mt-2">{{ progressText }}</p>
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
import { unzipSync } from "fflate";

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
      progressText: '',
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
    chooseAndImport() {
      // Both the browser dashboard and the desktop (Wails) webview pick the
      // .zip with a file input; the archive is unzipped and filtered in the
      // client and streamed tweet-by-tweet, so neither uploads the whole file.
      this.$refs.fileInput.click();
    },
    async onFileChange(e) {
      const file = e.target.files && e.target.files[0];
      e.target.value = ''; // allow re-selecting the same file later
      if (!file) return;
      await this.runBrowserImport(file);
    },
    // runBrowserImport unzips and filters the X archive in the browser and
    // streams only the kept original tweets (text + up to four photos) to the
    // node one at a time. Retweets, replies, GIFs and videos are dropped here
    // and never uploaded, so the node never buffers the whole archive.
    async runBrowserImport(file) {
      this.phase = 'importing';
      this.errorMessage = '';
      this.result = { imported_tweets: 0, imported_images: 0, skipped_tweets: 0 };
      this.progressText = 'Reading archive…';
      try {
        const buf = new Uint8Array(await file.arrayBuffer());

        // Decompress only tweets.js / tweets-part*.js here — never the media,
        // so a gigabyte of video in the archive is never even inflated.
        const tweetEntries = unzipSync(buf, { filter: (f) => this.isTweetsFile(f.name) });
        const archiveTweets = [];
        for (const name of Object.keys(tweetEntries)) {
          const text = new TextDecoder().decode(tweetEntries[name]);
          archiveTweets.push(...this.parseArchiveTweets(text));
        }

        // Keep originals only; collect the photo filenames they reference.
        const kept = [];
        const neededMedia = new Set();
        for (const at of archiveTweets) {
          const t = this.toKeptTweet(at);
          if (!t) { this.result.skipped_tweets++; continue; }
          kept.push(t);
          for (const fn of t.mediaFilenames) neededMedia.add(fn);
        }

        // Decompress only the referenced still photos — videos/GIFs are skipped.
        // fflate keys entries by their full archive path, so re-key by basename
        // to match the per-tweet media filenames (`<id>-<basename>`).
        const mediaByName = {};
        if (neededMedia.size > 0) {
          this.progressText = 'Extracting photos…';
          const entries = unzipSync(buf, { filter: (f) => neededMedia.has(this.basename(f.name)) });
          for (const name of Object.keys(entries)) {
            mediaByName[this.basename(name)] = entries[name];
          }
        }

        // Stream each kept tweet to the node, one at a time.
        for (let i = 0; i < kept.length; i++) {
          const t = kept[i];
          this.progressText = `Importing ${i + 1} / ${kept.length}…`;
          const images = [];
          for (const fn of t.mediaFilenames) {
            const bytes = mediaByName[fn];
            if (bytes) images.push(this.bytesToBase64(bytes));
          }
          const resp = await warpnetService.importTweet({
            id: t.id, text: t.text, createdAt: t.createdAt, images,
          });
          if (resp && resp.code) throw new Error(resp.message || 'Import failed');
          this.result.imported_tweets += resp?.imported_tweets || 0;
          this.result.imported_images += resp?.imported_images || 0;
          this.result.skipped_tweets += resp?.skipped_tweets || 0;
        }

        this.progressText = '';
        this.phase = 'done';
        this.$emit('imported', this.result);
      } catch (err) {
        console.error('Tweet import failed:', err);
        this.errorMessage = (err && err.message) ? err.message : 'Import failed. Please try again.';
        this.phase = 'error';
      }
    },
    isTweetsFile(name) {
      const base = this.basename(name);
      return base === 'tweets.js' || (base.startsWith('tweets-part') && base.endsWith('.js'));
    },
    basename(p) {
      const i = p.lastIndexOf('/');
      return i >= 0 ? p.slice(i + 1) : p;
    },
    // parseArchiveTweets strips the "window.YTD.tweets.partN = " assignment
    // wrapping the JSON array (everything from the first '[' is valid JSON).
    parseArchiveTweets(text) {
      const idx = text.indexOf('[');
      if (idx < 0) return [];
      return JSON.parse(text.slice(idx)).map((w) => w && w.tweet).filter(Boolean);
    },
    // toKeptTweet mirrors the node's scope rules: drop retweets ("RT @") and
    // replies (in_reply_to_status_id_str), keep only still photos (≤4), and
    // drop a tweet with neither text nor photos. Returns null when out of scope.
    toKeptTweet(at) {
      if (!at || !at.id_str) return null;
      const full = at.full_text || '';
      if (full.startsWith('RT @')) return null;
      if (at.in_reply_to_status_id_str) return null;
      const mediaFilenames = this.photoMedia(at)
        .slice(0, 4)
        .map((m) => at.id_str + '-' + this.basename(m.media_url_https));
      const text = this.htmlUnescape(this.stripMediaUrls(full, at)).trim();
      if (text === '' && mediaFilenames.length === 0) return null;
      return { id: at.id_str, text, createdAt: at.created_at || '', mediaFilenames };
    },
    // stripMediaUrls removes the t.co media short-links X appends to full_text.
    // The photo is imported separately (and GIFs/videos are dropped), so the
    // bare link would otherwise render as text — and dangle when the media is
    // absent from the archive. Non-media links (entities.urls) are left alone.
    stripMediaUrls(text, at) {
      const media = []
        .concat((at.extended_entities && at.extended_entities.media) || [])
        .concat((at.entities && at.entities.media) || []);
      let out = text;
      for (const m of media) {
        if (m && m.url) out = out.split(m.url).join('');
      }
      return out;
    },
    // photoMedia trusts extended_entities (authoritative for the real media
    // type) and falls back to entities; only "photo" survives.
    photoMedia(at) {
      let media = (at.extended_entities && at.extended_entities.media) || [];
      if (media.length === 0) media = (at.entities && at.entities.media) || [];
      return media.filter((m) => m.type === 'photo' && m.media_url_https);
    },
    htmlUnescape(s) {
      const el = document.createElement('textarea');
      el.innerHTML = s;
      return el.value;
    },
    // bytesToBase64 encodes in 32 KB chunks to avoid a per-byte string build
    // and the call-stack blowup of String.fromCharCode(...wholeArray).
    bytesToBase64(bytes) {
      let bin = '';
      const chunk = 0x8000;
      for (let i = 0; i < bytes.length; i += chunk) {
        bin += String.fromCharCode.apply(null, bytes.subarray(i, i + chunk));
      }
      return btoa(bin);
    },
  },
};
</script>
