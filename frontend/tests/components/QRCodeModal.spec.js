/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { mount } from "@vue/test-utils";
import QRCodeModal from "@/components/QRCodeModal.vue";

describe("QRCodeModal copy connection data", () => {
  beforeEach(() => {
    Object.assign(navigator, {
      clipboard: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
  });

  it("only offers the copy button when a raw payload is present", () => {
    const without = mount(QRCodeModal, {
      props: { show: true, qrData: "data:image/png;base64,x" },
    });
    expect(without.text()).not.toContain("Copy connection data");

    const withPayload = mount(QRCodeModal, {
      props: { show: true, qrData: "data:image/png;base64,x", qrPayload: "BASE45PAYLOAD" },
    });
    expect(withPayload.text()).toContain("Copy connection data");
  });

  it("copies the exact raw payload to the clipboard and flips the label", async () => {
    const wrapper = mount(QRCodeModal, {
      props: { show: true, qrData: "data:image/png;base64,x", qrPayload: "RAWCONNDATA123" },
    });
    await wrapper.find("button[aria-label^='Copy connection data']").trigger("click");
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith("RAWCONNDATA123");
    await wrapper.vm.$nextTick();
    expect(wrapper.text()).toContain("Copied!");
  });

  it("resets the label back to the idle state when the dialog closes", async () => {
    const wrapper = mount(QRCodeModal, {
      props: { show: true, qrData: "data:image/png;base64,x", qrPayload: "RAWCONNDATA123" },
    });
    await wrapper.find("button[aria-label^='Copy connection data']").trigger("click");
    await wrapper.vm.$nextTick();
    expect(wrapper.text()).toContain("Copied!");

    await wrapper.setProps({ show: false });
    await wrapper.setProps({ show: true });
    expect(wrapper.text()).toContain("Copy connection data");
    expect(wrapper.text()).not.toContain("Copied!");
  });
});
