/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach } from "vitest";
import { mount } from "@vue/test-utils";
import EmojiPicker from "@/components/EmojiPicker.vue";
import { loadRecent } from "@/lib/emoji";

const mountPicker = () => mount(EmojiPicker, { attachTo: document.body });

describe("EmojiPicker", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("opens on the first category and renders its emoji as buttons", () => {
    const wrapper = mountPicker();
    expect(wrapper.find("button[aria-label='grinning face smile happy']").exists()).toBe(true);
  });

  it("emits the picked character", async () => {
    const wrapper = mountPicker();
    await wrapper.find("button[aria-label='grinning face smile happy']").trigger("click");
    expect(wrapper.emitted("select")[0]).toEqual(["😀"]);
  });

  it("filters to matching emoji while searching", async () => {
    const wrapper = mountPicker();
    await wrapper.find("input[aria-label='Search emoji']").setValue("pizza");
    const buttons = wrapper.findAll(".grid button");
    expect(buttons).toHaveLength(1);
    expect(buttons[0].text()).toBe("🍕");
  });

  it("says so when nothing matches", async () => {
    const wrapper = mountPicker();
    await wrapper.find("input[aria-label='Search emoji']").setValue("zzzznotanemoji");
    expect(wrapper.text()).toContain("No emoji found");
  });

  it("remembers what was picked and offers it in a recents tab", async () => {
    const first = mountPicker();
    await first.find("button[aria-label='grinning face smile happy']").trigger("click");
    expect(loadRecent()[0][0]).toBe("😀");

    // A freshly opened picker starts on recents once there is history.
    const second = mountPicker();
    const recentTab = second.find("button[aria-label='Recently used']");
    expect(recentTab.exists()).toBe(true);
    expect(second.findAll(".grid button")[0].text()).toBe("😀");
  });

  it("applies the chosen skin tone to emoji that accept one", async () => {
    const wrapper = mountPicker();
    await wrapper.find("button[aria-label^='Skin tone']").trigger("click");
    await wrapper.find("button[aria-label='Dark']").trigger("click");

    await wrapper.find("button[aria-label='People & Body']").trigger("click");
    await wrapper.find("button[aria-label^='thumbs up sign']").trigger("click");
    expect(wrapper.emitted("select")[0]).toEqual(["👍\u{1F3FF}"]);
  });

  it("leaves emoji that take no skin tone unchanged", async () => {
    const wrapper = mountPicker();
    await wrapper.find("button[aria-label^='Skin tone']").trigger("click");
    await wrapper.find("button[aria-label='Dark']").trigger("click");

    await wrapper.find("input[aria-label='Search emoji']").setValue("pizza");
    await wrapper.find(".grid button").trigger("click");
    expect(wrapper.emitted("select")[0]).toEqual(["🍕"]);
  });

  it("stores a recent with its tone already applied so it is not toned twice", async () => {
    const wrapper = mountPicker();
    await wrapper.find("button[aria-label^='Skin tone']").trigger("click");
    await wrapper.find("button[aria-label='Dark']").trigger("click");
    await wrapper.find("button[aria-label='People & Body']").trigger("click");
    await wrapper.find("button[aria-label='waving hand sign hello hi bye wave']").trigger("click");

    const reopened = mountPicker();
    expect(reopened.findAll(".grid button")[0].text()).toBe("👋\u{1F3FF}");
  });

  it("closes on Escape", async () => {
    const wrapper = mountPicker();
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await wrapper.vm.$nextTick();
    expect(wrapper.emitted("close")).toBeTruthy();
  });

  it("closes when the click lands outside the picker", async () => {
    const wrapper = mountPicker();
    document.body.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
    await wrapper.vm.$nextTick();
    expect(wrapper.emitted("close")).toBeTruthy();
  });

  it("stays open while clicking inside itself", async () => {
    const wrapper = mountPicker();
    wrapper.find("input[aria-label='Search emoji']").element
      .dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
    await wrapper.vm.$nextTick();
    expect(wrapper.emitted("close")).toBeFalsy();
  });

  // The button that opens the picker sits in the anchor wrapper. If the
  // outside-click handler treated it as "outside", it would close the picker
  // on mousedown and the button's own click would immediately reopen it.
  it("treats the button that opened it as inside the picker", async () => {
    const host = {
      components: { EmojiPicker },
      template: `
        <div data-emoji-anchor>
          <button id="toggle" type="button"></button>
          <EmojiPicker @close="onClose" />
        </div>`,
      data: () => ({ closed: 0 }),
      methods: {
        onClose() {
          this.closed++;
        },
      },
    };
    const wrapper = mount(host, { attachTo: document.body });
    wrapper.find("#toggle").element.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
    await wrapper.vm.$nextTick();
    expect(wrapper.vm.closed).toBe(0);

    document.body.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
    await wrapper.vm.$nextTick();
    expect(wrapper.vm.closed).toBe(1);
  });
});
