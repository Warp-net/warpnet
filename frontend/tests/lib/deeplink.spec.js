/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect } from "vitest";
import { parseDeepLink } from "@/lib/deeplink";

describe("parseDeepLink", () => {
  it("parses the happy path", () => {
    const got = parseDeepLink("warpnet://user/01HZX7K8");
    expect(got).toEqual({ kind: "user", id: "01HZX7K8", raw: "warpnet://user/01HZX7K8" });
  });

  it("tolerates trailing slash", () => {
    expect(parseDeepLink("warpnet://user/abc/")).toMatchObject({ kind: "user", id: "abc" });
  });

  it("tolerates the shell-stripped no-slashes form", () => {
    expect(parseDeepLink("warpnet:user/abc")).toMatchObject({ kind: "user", id: "abc" });
  });

  it("tolerates uppercase scheme", () => {
    expect(parseDeepLink("WARPNET://user/abc")).toMatchObject({ kind: "user", id: "abc" });
  });

  it("decodes percent-encoded ids", () => {
    expect(parseDeepLink("warpnet://user/a%20b")).toMatchObject({ id: "a b" });
  });

  it("rejects missing id", () => {
    expect(parseDeepLink("warpnet://user/")).toBeNull();
    expect(parseDeepLink("warpnet://user")).toBeNull();
  });

  it("rejects unsupported kinds", () => {
    expect(parseDeepLink("warpnet://tweet/123")).toBeNull();
  });

  it("rejects foreign schemes", () => {
    expect(parseDeepLink("https://example.com/")).toBeNull();
  });

  it("rejects blank and garbage", () => {
    expect(parseDeepLink("")).toBeNull();
    expect(parseDeepLink(null)).toBeNull();
    expect(parseDeepLink(undefined)).toBeNull();
    expect(parseDeepLink("not a url")).toBeNull();
  });
});
