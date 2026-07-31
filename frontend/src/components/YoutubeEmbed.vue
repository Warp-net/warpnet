<!-- Warpnet - Decentralized Social Network -->
<template>
  <div class="mt-2 rounded-lg border border-lighter overflow-hidden">
    <!--
      Facade first: no request reaches YouTube (not even for a thumbnail)
      until the user presses play, so merely scrolling past a link doesn't
      disclose the reader to Google.
    -->
    <div v-if="!playing" class="relative bg-lighter">
      <button
          @click.stop="playing = true"
          type="button"
          class="w-full py-8 flex flex-col items-center justify-center text-dark hover:bg-lightblue transition-colors"
          :aria-label="`Play YouTube video ${videoId}`"
      >
        <i class="fab fa-youtube text-4xl mb-2" style="color:#ff0000" aria-hidden="true"></i>
        <span class="text-sm font-semibold">Play YouTube video</span>
        <span class="text-xs mt-1">Nothing is sent to YouTube until you press play</span>
      </button>
      <a
          :href="watchUrl"
          target="_blank"
          rel="noopener noreferrer nofollow"
          class="block text-center text-xs text-blue pb-2 hover:underline"
          @click.stop
      >Open on YouTube</a>
    </div>
    <div v-else class="relative w-full" style="padding-top:56.25%">
      <iframe
          :src="embedUrl"
          class="absolute inset-0 w-full h-full"
          frameborder="0"
          allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
          allowfullscreen
          referrerpolicy="strict-origin-when-cross-origin"
          title="YouTube video player"
      ></iframe>
    </div>
  </div>
</template>

<script>
import {youtubeEmbedUrl, youtubeWatchUrl} from "@/lib/youtube";

export default {
  name: "YoutubeEmbed",
  props: {
    videoId: {type: String, required: true},
  },
  data() {
    return {
      playing: false,
    };
  },
  computed: {
    embedUrl() {
      return youtubeEmbedUrl(this.videoId);
    },
    watchUrl() {
      return youtubeWatchUrl(this.videoId);
    },
  },
};
</script>
