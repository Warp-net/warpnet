<!-- SPDX-License-Identifier: AGPL-3.0-or-later -->
<template>
  <div
    v-if="state.isActive"
    class="fixed inset-0 z-[200]"
    role="dialog"
    aria-modal="true"
    aria-labelledby="onboarding-title"
    data-testid="onboarding"
  >
    <!-- One box both dims the page and rings the element it points at, both in
         its box-shadow: a ring-* class would be overwritten by the inline one. -->
    <div class="fixed pointer-events-none rounded-xl transition-all duration-150" :style="spotlightStyle"></div>
    <div
      ref="card"
      class="fixed w-[20rem] max-w-[calc(100vw-2rem)] rounded-lg shadow-md border border-lighter bg-white dark:bg-darktheme-card p-4"
      :style="cardStyle"
    >
      <h2 id="onboarding-title" class="font-bold text-lg leading-tight">{{ step.title }}</h2>
      <p class="text-sm mt-2">{{ step.text }}</p>
      <div class="flex items-center justify-between mt-4">
        <button v-if="!isLast" type="button" class="text-sm text-dark hover:underline flat-btn" @click="finish">
          Skip tour
        </button>
        <span v-else></span>
        <div class="flex items-center gap-2">
          <button
            v-if="state.index > 0"
            type="button"
            class="px-3 py-1.5 rounded-full text-sm border border-lighter hover:bg-lighter"
            @click="move(-1)"
          >
            Back
          </button>
          <button
            ref="next"
            type="button"
            class="px-4 py-1.5 rounded-full text-sm font-semibold text-white bg-blue hover:bg-darkblue"
            @click="move(1)"
          >
            {{ isLast ? "Done" : "Next" }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import {finishOnboarding, moveOnboarding, onboarding, onboardingSteps} from "@/lib/onboarding";

// An anchor can belong to a view that is still mounting; past this wait the
// tip is skipped as one that does not apply here.
const anchorWaitMs = 800;
const gap = 12;
const margin = 16;
const spotlightPad = 4;

function visibleAnchor(name) {
  return [...document.querySelectorAll(`[data-tour="${name}"]`)].find((el) => {
    const r = el.getBoundingClientRect();
    return r.width > 0 && r.height > 0;
  });
}

export default {
  name: "OnboardingTour",
  data() {
    return {
      state: onboarding,
      rect: null,
      isWaiting: false,
      waitSince: 0,
      cardPos: null,
      frame: 0,
      scrolledFor: -1,
      focusedFor: -1,
    };
  },
  computed: {
    step() {
      return onboardingSteps[this.state.index];
    },
    isLast() {
      return this.state.index === onboardingSteps.length - 1;
    },
    spotlightStyle() {
      // Chromium drops a box-shadow spread as large as 9999px, so it is sized to the window.
      const shade = "0 0 0 200vmax rgba(0, 0, 0, 0.7)";
      if (!this.rect) {
        return {top: "50%", left: "50%", width: "0px", height: "0px", boxShadow: shade};
      }
      return {
        top: `${this.rect.top - spotlightPad}px`,
        left: `${this.rect.left - spotlightPad}px`,
        width: `${this.rect.width + spotlightPad * 2}px`,
        height: `${this.rect.height + spotlightPad * 2}px`,
        boxShadow: `0 0 0 2px var(--gd-trim), 0 0 14px 4px var(--gd-trim), ${shade}`,
      };
    },
    cardStyle() {
      // Kept hidden until it has a place, so it never flashes in the middle.
      if (this.isWaiting || (this.rect && !this.cardPos)) {
        return {top: "0px", left: "0px", visibility: "hidden"};
      }
      if (!this.cardPos) {
        return {top: "50%", left: "50%", transform: "translate(-50%, -50%)"};
      }
      return {top: `${this.cardPos.top}px`, left: `${this.cardPos.left}px`};
    },
  },
  watch: {
    "state.isActive": {
      immediate: true,
      handler(isActive) {
        cancelAnimationFrame(this.frame);
        if (isActive) {
          this.scrolledFor = -1;
          this.focusedFor = -1;
          this.resetWait();
          this.frame = requestAnimationFrame(this.track);
        }
      },
    },
    "state.index"() {
      this.resetWait();
    },
  },
  mounted() {
    this._onKeyup = (e) => {
      if (!this.state.isActive) return;
      if (e.key === "Escape") this.finish();
      if (e.key === "ArrowRight") this.move(1);
      if (e.key === "ArrowLeft") this.move(-1);
    };
    window.addEventListener("keyup", this._onKeyup);
  },
  beforeUnmount() {
    cancelAnimationFrame(this.frame);
    window.removeEventListener("keyup", this._onKeyup);
  },
  methods: {
    move(direction) {
      moveOnboarding(direction);
    },
    finish() {
      finishOnboarding();
    },
    resetWait() {
      this.isWaiting = !!this.step?.anchor;
      this.waitSince = Date.now();
      this.rect = null;
      this.cardPos = null;
    },
    // Followed every frame rather than measured once: the page under the tour
    // keeps moving — avatars load, banners come and go, the window resizes.
    track() {
      if (!this.state.isActive) return;
      const index = this.state.index;
      const anchor = this.step.anchor;
      const el = anchor ? visibleAnchor(anchor) : null;
      if (anchor && !el) {
        if (Date.now() - this.waitSince > anchorWaitMs) {
          moveOnboarding(this.state.direction);
        }
      } else {
        if (el && this.scrolledFor !== index) {
          this.scrolledFor = index;
          el.scrollIntoView({block: "nearest", inline: "nearest"});
        }
        this.isWaiting = false;
        this.setRect(el ? el.getBoundingClientRect() : null);
        this.placeCard();
        if (this.focusedFor !== index) {
          this.focusedFor = index;
          this.$nextTick(() => this.$refs.next?.focus());
        }
      }
      this.frame = requestAnimationFrame(this.track);
    },
    setRect(r) {
      const next = r ? {top: r.top, left: r.left, width: r.width, height: r.height} : null;
      const prev = this.rect;
      if (prev === next) return;
      if (prev && next && prev.top === next.top && prev.left === next.left &&
          prev.width === next.width && prev.height === next.height) {
        return;
      }
      this.rect = next;
    },
    // Right of the element first (the side rail), then below, above, left;
    // whatever wins is clamped into the window.
    placeCard() {
      const card = this.$refs.card;
      if (!this.rect || !card) {
        if (this.cardPos) this.cardPos = null;
        return;
      }
      const w = card.offsetWidth;
      const h = card.offsetHeight;
      const vw = window.innerWidth;
      const vh = window.innerHeight;
      const r = this.rect;
      const candidates = [
        {top: r.top, left: r.left + r.width + gap, fits: r.left + r.width + gap + w <= vw - margin},
        {top: r.top + r.height + gap, left: r.left, fits: r.top + r.height + gap + h <= vh - margin},
        {top: r.top - gap - h, left: r.left, fits: r.top - gap - h >= margin},
        {top: r.top, left: r.left - gap - w, fits: r.left - gap - w >= margin},
      ];
      const pick = candidates.find((c) => c.fits) || candidates[1];
      const top = Math.round(Math.min(Math.max(pick.top, margin), Math.max(margin, vh - h - margin)));
      const left = Math.round(Math.min(Math.max(pick.left, margin), Math.max(margin, vw - w - margin)));
      if (this.cardPos && this.cardPos.top === top && this.cardPos.left === left) return;
      this.cardPos = {top, left};
    },
  },
};
</script>
