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

  it("copies the exact payload to the clipboard verbatim and flips the label", async () => {
    const json = '{"node_id":"12D3KooWAbc","user_id":"01H0","token":"t"}';
    const wrapper = mount(QRCodeModal, {
      props: { show: true, qrData: "data:image/png;base64,x", qrPayload: json },
    });
    await wrapper.find("button[aria-label^='Copy connection data']").trigger("click");
    // Copied verbatim — plain JSON in, plain JSON out (no compression/encoding).
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(json);
    const copied = navigator.clipboard.writeText.mock.calls[0][0];
    expect(() => JSON.parse(copied)).not.toThrow();
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
