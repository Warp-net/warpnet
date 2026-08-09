<template>
  <div class="mt-1 mb-2">
    <div v-for="(option, index) in options" :key="index" class="mb-2">
      <button
          v-if="!showResults"
          @click.stop="vote(index)"
          type="button"
          class="w-full text-left px-4 py-2 rounded-full border border-blue text-blue font-semibold text-sm hover:bg-lightblue transition-colors flat-btn"
          :class="voting ? 'opacity-50 cursor-not-allowed' : ''"
          :disabled="voting"
      >
        {{ option }}
      </button>
      <div v-else class="relative rounded border border-lighter overflow-hidden">
        <!-- A translucent brand tint, not a solid light fill: the themed
             surface shows through, so the label keeps its contrast in the
             dark and mastodon themes as well as the light one. -->
        <div class="absolute inset-y-0 left-0 bg-blue bg-opacity-25" :style="{width: percent(index) + '%'}"></div>
        <div class="relative flex items-center justify-between px-3 py-2 text-sm">
          <span :class="votedOption === index ? 'font-semibold' : ''">
            <i v-if="votedOption === index" class="fas fa-check-circle text-blue mr-1" aria-hidden="true"></i>
            {{ option }}
          </span>
          <span class="text-dark ml-2">{{ percent(index) }}%</span>
        </div>
      </div>
    </div>
    <p class="text-xs text-dark">
      {{ totalVotes }} {{ totalVotes === 1 ? 'vote' : 'votes' }} · {{ statusLabel }}
    </p>
  </div>
</template>

<script>
import {warpnetService} from "@/service/service";
import {toast} from "@/lib/toast";

export default {
  name: "PollWidget",
  props: {
    tweet: {type: Object, required: true},
  },
  data() {
    return {
      votes: [],
      totalVotes: 0,
      votedOption: null,
      voting: false,
    };
  },
  computed: {
    options() {
      return (this.tweet.poll && this.tweet.poll.options) || [];
    },
    closed() {
      const expiresAt = this.tweet.poll && this.tweet.poll.expires_at;
      if (!expiresAt) return false;
      return new Date(expiresAt).getTime() <= Date.now();
    },
    // Same rule as Twitter: the tally stays hidden until you've had your say
    // or the poll has closed, so early results can't steer later voters.
    showResults() {
      return this.closed || this.votedOption !== null;
    },
    statusLabel() {
      if (this.closed) return 'Final results';
      const remaining = new Date(this.tweet.poll.expires_at).getTime() - Date.now();
      const hours = Math.floor(remaining / 3600000);
      if (hours >= 24) return `${Math.floor(hours / 24)}d left`;
      if (hours >= 1) return `${hours}h left`;
      return `${Math.max(1, Math.round(remaining / 60000))}m left`;
    },
  },
  mounted() {
    this.loadResults();
  },
  methods: {
    percent(index) {
      if (!this.totalVotes) return 0;
      return Math.round(((this.votes[index] || 0) / this.totalVotes) * 100);
    },
    applyResults(results) {
      if (!results || !Array.isArray(results.votes)) return false;
      this.votes = results.votes;
      this.totalVotes = results.total_votes || 0;
      this.votedOption = Number.isInteger(results.voted_option) ? results.voted_option : null;
      return true;
    },
    async loadResults() {
      if (this.options.length === 0) return;
      try {
        const results = await warpnetService.getPollResults(
            this.tweet.id, this.tweet.user_id, this.options.length,
        );
        this.applyResults(results);
      } catch (err) {
        console.error(`failed to load poll results [${this.tweet.id}]`, err);
      }
    },
    async vote(index) {
      if (this.voting || this.showResults) return;
      this.voting = true;
      try {
        const results = await warpnetService.voteInPoll(
            this.tweet.id, this.tweet.user_id, index, this.options.length,
        );
        // The transport turns a node-side failure into an empty body, so an
        // unusable answer must leave the choices clickable rather than
        // showing an empty tally as if the vote had landed.
        if (!this.applyResults(results)) {
          toast.error("Couldn't cast your vote. Please try again.");
          return;
        }
        // A vote is final, so reveal the results even if the node answered
        // without echoing our choice back.
        if (this.votedOption === null) this.votedOption = index;
      } catch (err) {
        console.error(`failed to vote in poll [${this.tweet.id}]`, err);
        toast.error(err?.message || "Couldn't cast your vote. Please try again.");
      } finally {
        this.voting = false;
      }
    },
  },
};
</script>
