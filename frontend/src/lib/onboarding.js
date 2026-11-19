// SPDX-License-Identifier: AGPL-3.0-or-later

// First-run tour: a chain of tips pinned to elements marked data-tour="<anchor>".
// OnboardingTour.vue (mounted once in App.vue) renders it and skips a tip whose
// element is not on screen — the wallet off testnet, the right bar on a phone.
import { reactive } from "vue";

const pendingStorage = "warpnet.onboarding.pending";

export const onboardingSteps = [
  {
    anchor: "",
    title: "Welcome to Warpnet",
    text: "There is no central server here. This app is your own node: your tweets, follows and messages live on this computer and travel straight between nodes. Let's take a quick look around.",
  },
  {
    anchor: "network",
    title: "Test network",
    text: "You are on a test network. Anything here may be wiped, so try things freely.",
  },
  {
    anchor: "node-info",
    title: "Your node",
    text: "Your node's ID, network, addresses and rating. Other nodes find and reach you by them.",
  },
  {
    anchor: "compose",
    title: "Write a tweet",
    text: "A tweet is saved on your node and sent to your followers' nodes. Up to 2048 bytes.",
  },
  {
    anchor: "compose-tools",
    title: "Add to a tweet",
    text: "Attach images or a video, start a poll, add emoji — or make the tweet sponsored: readers pay USDT once to unlock it.",
  },
  {
    anchor: "import",
    title: "Bring your history",
    text: "Import tweets from your X archive so your profile does not start empty.",
  },
  {
    anchor: "search",
    title: "Find people",
    text: "Search by name or user ID. A handle like @user@mastodon.social finds a Fediverse account.",
  },
  {
    anchor: "who-to-follow",
    title: "Who to follow",
    text: "People your node has met. The other tabs show Mastodon and Threads accounts bridged into Warpnet. A green dot means that user's node is online now, red — offline.",
  },
  {
    anchor: "nav-home",
    title: "Home",
    text: "Tweets from you and the people you follow.",
  },
  {
    anchor: "nav-notifications",
    title: "Notifications",
    text: "Replies, likes, retweets, follows and mentions. A badge counts the unread ones.",
  },
  {
    anchor: "nav-messages",
    title: "Messages",
    text: "Private chats, sent straight to the other person's node. If they are offline, your node delivers the message when they are back.",
  },
  {
    anchor: "nav-bookmarks",
    title: "Bookmarks",
    text: "Tweets you saved for later. They stay on your node and only you see them.",
  },
  {
    anchor: "nav-wallet",
    title: "Wallet",
    text: "TRX and USDT on the TRON test network: pay for sponsored tweets and get paid for yours. A badge counts new incoming transfers.",
  },
  {
    anchor: "nav-profile",
    title: "Profile",
    text: "Your page: avatar, bio, tweets and followers. Fill it in so people recognise you.",
  },
  {
    anchor: "nav-settings",
    title: "Settings",
    text: "Filters, mutes and blocks, paired phones, the Fediverse gateway, network and your node's rating.",
  },
  {
    anchor: "tweet-button",
    title: "Tweet from anywhere",
    text: "Opens the composer from any page.",
  },
  {
    anchor: "account-menu",
    title: "Your account",
    text: "Switch the theme, pair your phone by QR code, replay this tour, or sign out. Signing out stops your node.",
  },
  {
    anchor: "account-menu",
    title: "Pair your phone",
    text: "Sign in by QR code and scan it with the Warpnet mobile app. The phone then reads and posts as you through this node. Its top bar shows a monitor icon: green — connected to this node, red — this node is unreachable, so keep this app running.",
  },
  {
    anchor: "",
    title: "You're all set",
    text: "Follow a few people to fill your timeline.",
  },
];

export const onboarding = reactive({ isActive: false, index: 0, direction: 1 });

// Sign-up queues the tour and Home starts it once the timeline has loaded, so
// it never opens over the login screen or a half-drawn page.
export function queueOnboarding() {
  try {
    localStorage.setItem(pendingStorage, "1");
  } catch (e) {}
}

export function startOnboarding() {
  onboarding.index = 0;
  onboarding.direction = 1;
  onboarding.isActive = true;
}

export function startQueuedOnboarding() {
  let isQueued = false;
  try {
    isQueued = localStorage.getItem(pendingStorage) === "1";
  } catch (e) {}
  if (isQueued && !onboarding.isActive) {
    startOnboarding();
  }
}

export function finishOnboarding() {
  onboarding.isActive = false;
  try {
    localStorage.removeItem(pendingStorage);
  } catch (e) {}
}

export function moveOnboarding(direction) {
  const next = onboarding.index + direction;
  if (next < 0) {
    return;
  }
  if (next >= onboardingSteps.length) {
    finishOnboarding();
    return;
  }
  onboarding.direction = direction;
  onboarding.index = next;
}
