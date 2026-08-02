/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, vi } from "vitest";
import { mount } from "@vue/test-utils";

vi.mock("@/service/service", () => ({
  warpnetService: { editTweet: vi.fn() },
}));

import EditTweetOverlay from "@/components/EditTweetOverlay.vue";

// Covers the wiring shared by every composer: the component's insertEmoji
// method calls the same-named helper imported from @/lib/emoji, so a scoping
// slip there would recurse instead of inserting.
const mountOverlay = (text = "") => {
  const wrapper = mount(EditTweetOverlay, {
    props: { show: true, tweet: { id: "t1", text } },
    global: { stubs: { EmojiPicker: true } },
    attachTo: document.body,
  });
  return wrapper;
};

describe("EditTweetOverlay emoji wiring", () => {
  it("appends the emoji when the caret has not moved", async () => {
    const wrapper = mountOverlay("hello");
    wrapper.vm.insertEmoji("😀");
    await wrapper.vm.$nextTick();
    expect(wrapper.vm.text).toBe("hello😀");
  });

  it("inserts at the caret position", async () => {
    const wrapper = mountOverlay("ab");
    const field = wrapper.find("textarea").element;
    field.setSelectionRange(1, 1);
    wrapper.vm.insertEmoji("😀");
    await wrapper.vm.$nextTick();
    expect(wrapper.vm.text).toBe("a😀b");
  });

  it("counts the tweet in runes, not UTF-16 units", async () => {
    const wrapper = mountOverlay("😀😀😀");
    await wrapper.vm.$nextTick();
    expect(wrapper.vm.textLength).toBe(3);
    expect(wrapper.text()).toContain("3 / 280");
  });

  it("refuses an emoji that would push the tweet past 280 runes", async () => {
    const wrapper = mountOverlay("a".repeat(280));
    wrapper.vm.insertEmoji("😀");
    await wrapper.vm.$nextTick();
    expect(wrapper.vm.text).toBe("a".repeat(280));
  });

  it("clamps pasted text on the rune count", async () => {
    const wrapper = mountOverlay("");
    // 300 emoji are 600 UTF-16 units; the clamp must leave 280 emoji, not 140.
    wrapper.vm.text = "😀".repeat(300);
    await wrapper.vm.$nextTick();
    expect(wrapper.vm.textLength).toBe(280);
    expect(wrapper.vm.text).not.toContain("�");
  });

  it("lets Escape close the picker without discarding the draft", async () => {
    const wrapper = mountOverlay("draft");
    wrapper.vm.showEmojiPicker = true;
    await wrapper.vm.$nextTick();
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await wrapper.vm.$nextTick();
    expect(wrapper.emitted("close")).toBeFalsy();

    wrapper.vm.showEmojiPicker = false;
    await wrapper.vm.$nextTick();
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await wrapper.vm.$nextTick();
    expect(wrapper.emitted("close")).toBeTruthy();
  });
});
