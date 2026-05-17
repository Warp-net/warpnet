/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("../../wailsjs/go/main/App", () => ({
    Call: vi.fn(),
}));

import { Call as callMock } from "../../wailsjs/go/main/App";
import { warpnetService, PRIVATE_POST_TWEET, PUBLIC_GET_TWEET } from "@/service/service";

describe("warpnetService.sendToNode idempotency dedup", () => {
    beforeEach(() => {
        callMock.mockReset();
        warpnetService.setOwnerProfile({ user_id: "u1", node_id: "n1" });
    });

    it("collapses concurrent identical POST requests into a single backend call", async () => {
        let resolveCall;
        callMock.mockImplementationOnce(
            () => new Promise((r) => { resolveCall = r; })
        );

        const body = { user_id: "u1", text: "hello world" };
        const p1 = warpnetService.sendToNode({ path: PRIVATE_POST_TWEET, body });
        const p2 = warpnetService.sendToNode({ path: PRIVATE_POST_TWEET, body });

        expect(callMock).toHaveBeenCalledTimes(1);

        resolveCall({ body: { tweet_id: "t1" } });
        const [r1, r2] = await Promise.all([p1, p2]);
        expect(r1).toEqual({ tweet_id: "t1" });
        expect(r2).toEqual({ tweet_id: "t1" });
    });

    it("does not dedup non-POST requests", async () => {
        callMock.mockResolvedValue({ body: { ok: true } });

        const body = { user_id: "u1", tweet_id: "t1" };
        await Promise.all([
            warpnetService.sendToNode({ path: PUBLIC_GET_TWEET, body }),
            warpnetService.sendToNode({ path: PUBLIC_GET_TWEET, body }),
        ]);

        expect(callMock).toHaveBeenCalledTimes(2);
    });

    it("releases the in-flight slot after the request resolves", async () => {
        callMock.mockResolvedValue({ body: { ok: true } });

        const body = { user_id: "u1", text: "hello world" };
        await warpnetService.sendToNode({ path: PRIVATE_POST_TWEET, body });
        await warpnetService.sendToNode({ path: PRIVATE_POST_TWEET, body });

        expect(callMock).toHaveBeenCalledTimes(2);
    });

    it("treats different bodies as distinct requests", async () => {
        let resolves = [];
        callMock.mockImplementation(
            () => new Promise((r) => { resolves.push(r); })
        );

        const p1 = warpnetService.sendToNode({
            path: PRIVATE_POST_TWEET,
            body: { user_id: "u1", text: "first" },
        });
        const p2 = warpnetService.sendToNode({
            path: PRIVATE_POST_TWEET,
            body: { user_id: "u1", text: "second" },
        });

        expect(callMock).toHaveBeenCalledTimes(2);

        resolves[0]({ body: { tweet_id: "t1" } });
        resolves[1]({ body: { tweet_id: "t2" } });
        expect(await p1).toEqual({ tweet_id: "t1" });
        expect(await p2).toEqual({ tweet_id: "t2" });
    });
});
