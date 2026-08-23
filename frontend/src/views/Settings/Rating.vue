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
        <h1 class="text-xl font-bold ml-4">Node rating</h1>
      </div>

      <Loader :loading="loading" />

      <div v-if="!loading && failed" class="px-5 py-6">
        <p class="font-bold text-lg">Rating is not available</p>
        <p class="text-sm text-dark">
          This node could not read its rating. It keeps working either way.
        </p>
      </div>

      <template v-if="!loading && !failed">
        <div class="px-5 py-6 border-b border-lighter">
          <div class="flex items-end">
            <p class="text-5xl font-bold" :class="bandColor(rating.band)">{{ rating.overall }}</p>
            <p class="text-dark text-lg ml-2 mb-1">/ 1000</p>
          </div>
          <p class="mt-1 font-bold" :class="bandColor(rating.band)">{{ bandLabel(rating.band) }}</p>
          <p class="text-sm text-dark mt-3">
            Your node does not rate itself — this is what
            <span class="font-bold">{{ rating.observers }}</span>
            {{ rating.observers === 1 ? 'other node has' : 'other nodes have' }}
            observed. It recovers on its own as older observations age out.
          </p>
          <p v-if="rating.mode === 'shadow'" class="text-sm text-dark mt-2">
            <i class="fas fa-flask mr-1"></i>
            This node is in shadow mode: the rating is measured and shown, but
            nothing acts on it yet.
          </p>
        </div>

        <div
          v-if="!rating.dimensions || rating.dimensions.length === 0"
          class="flex flex-col items-center justify-center pt-10 px-5"
        >
          <p class="font-bold text-lg">Nothing to report</p>
          <p class="text-sm text-dark text-center">
            No node has anything to say about yours. That is the best possible result.
          </p>
        </div>

        <div
          v-for="dim in rating.dimensions"
          :key="dim.name"
          class="px-5 py-4 border-b border-lighter"
        >
          <div class="flex items-center">
            <p class="font-bold">{{ dimensionLabel(dim.name) }}</p>
            <p class="ml-auto font-bold" :class="bandColor(dim.band)">{{ dim.score }}</p>
          </div>
          <p class="text-sm text-dark">{{ dimensionHint(dim.name) }}</p>

          <div class="mt-2 h-2 w-full rounded-full bg-lighter overflow-hidden">
            <div
              class="h-full rounded-full"
              :class="barColor(dim.band)"
              :style="{ width: barWidth(dim.score) }"
            ></div>
          </div>

          <div v-if="dim.recent && dim.recent.length" class="mt-3">
            <p class="text-sm font-bold text-dark">Recently observed</p>
            <div
              v-for="tally in dim.recent"
              :key="tally.kind"
              class="flex items-center text-sm py-1"
            >
              <span>{{ offenceLabel(tally.kind) }}</span>
              <span class="ml-auto text-dark">{{ tally.count }}&times;</span>
            </div>
          </div>
        </div>
      </template>
    </div>
    <DefaultRightBar :profile="ownerProfile" />
  </div>
</template>

<script>
import {defineAsyncComponent} from "vue";
import {warpnetService} from "@/service/service";

// Offence names come from the node's own catalogue. Anything not
// listed is shown as-is rather than hidden, so a node running a newer
// build still tells its user something useful.
const OFFENCE_LABELS = {
  bad_signature: "Messages with an invalid signature",
  missing_signature: "Messages sent unsigned",
  malformed_frame: "Malformed messages",
  oversize_payload: "Oversized payloads",
  stale_or_replayed: "Stale or replayed messages",
  private_route_denied: "Requests to private routes",
  rate_limit_hit: "Requests over the rate limit",
  discovery_flood: "Excessive peer discovery",
  connection_flap: "Repeated reconnections",
  dial_failure: "Failed connection attempts",
  forged_observation: "Invalid rating records",
  moderation_upheld: "Moderated content",
  foreign_authorship: "Posts on behalf of another user",
  write_flood: "Excessive posting",
  false_report_burst: "Reports found groundless",
  verdict_bad_signature: "Verdicts with an invalid signature",
  verdict_no_moderator_id: "Verdicts without a moderator id",
  verdict_malformed: "Malformed verdicts",
  verdict_unsolicited: "Verdicts for rounds not assigned",
  verdict_outlier: "Verdicts against the quorum",
  audit_wrong: "Failed moderation spot-checks",
  audit_invalid: "Invalid spot-check answers",
  audit_unreachable: "Unanswered spot-checks",
};

const DIMENSIONS = {
  net: {
    label: "Network",
    hint: "How this node behaves on the wire: signatures, message framing, request volume.",
  },
  app: {
    label: "Application",
    hint: "Content and posting behaviour, including upheld moderation decisions.",
  },
  mod: {
    label: "Moderation",
    hint: "For moderator nodes: the quality and validity of the verdicts they cast.",
  },
};

export default {
  name: "SettingsRating",
  components: {
    SideNav: defineAsyncComponent(() => import('@/components/SideNav.vue')),
    DefaultRightBar: defineAsyncComponent(() => import('@/components/DefaultRightBar.vue')),
    Loader: defineAsyncComponent(() => import('@/components/Loader.vue')),
  },
  data() {
    return {
      loading: true,
      failed: false,
      ownerProfile: {},
      rating: {
        overall: 1000,
        band: 'trusted',
        observers: 0,
        dimensions: [],
        mode: 'shadow',
      },
    };
  },
  methods: {
    bandLabel(band) {
      switch (band) {
        case 'trusted': return 'Trusted';
        case 'watched': return 'Watched';
        case 'degraded': return 'Degraded';
        case 'floor': return 'Severely degraded';
        default: return band;
      }
    },
    bandColor(band) {
      switch (band) {
        case 'watched': return 'text-yellow-500';
        case 'degraded': return 'text-orange-500';
        case 'floor': return 'text-red-500';
        default: return 'text-green-500';
      }
    },
    barColor(band) {
      switch (band) {
        case 'watched': return 'bg-yellow-500';
        case 'degraded': return 'bg-orange-500';
        case 'floor': return 'bg-red-500';
        default: return 'bg-green-500';
      }
    },
    barWidth(score) {
      const clamped = Math.max(0, Math.min(1000, Number(score) || 0));
      return `${clamped / 10}%`;
    },
    dimensionLabel(name) {
      return DIMENSIONS[name]?.label || name;
    },
    dimensionHint(name) {
      return DIMENSIONS[name]?.hint || '';
    },
    offenceLabel(kind) {
      return OFFENCE_LABELS[kind] || kind;
    },
  },
  async created() {
    this.ownerProfile = warpnetService.getOwnerProfile();
    try {
      const resp = await warpnetService.getOwnRating();
      if (!resp || resp.overall === undefined) {
        this.failed = true;
      } else {
        this.rating = resp;
      }
    } catch (err) {
      console.error('Failed to load node rating:', err);
      this.failed = true;
    } finally {
      this.loading = false;
    }
  },
};
</script>
