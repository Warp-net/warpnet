import Foundation
@testable import WarpnetTransport
import XCTest

final class WarpnetDtosTests: XCTestCase {
  func testTweetDecodesWithMissingAndNullFields() throws {
    let tweet = try WarpnetJSON.decode(WarpnetTweet.self, from: #"{"id":"t1","text":"hi","user_id":"u1","username":"alice","created_at":"2026-10-17T08:00:00Z","image_keys":null}"#)
    XCTAssertEqual(tweet.id, "t1")
    XCTAssertEqual(tweet.rootId, "")
    XCTAssertNil(tweet.parentId)
    XCTAssertNil(tweet.imageKeys)
    XCTAssertFalse(tweet.pinned)
    XCTAssertEqual(tweet.createdAt, "2026-10-17T08:00:00Z")
  }

  func testDraftTweetLeavesTimestampsOut() throws {
    let draft = WarpnetTweet(id: "", rootId: "r1", text: "reply", userId: "u1", username: "alice")
    let json = try XCTUnwrap(JSONSerialization.jsonObject(with: Data(WarpnetJSON.encode(draft).utf8)) as? [String: Any])
    XCTAssertNil(json["created_at"])
    XCTAssertNil(json["parent_id"])
    XCTAssertEqual(json["root_id"] as? String, "r1")
    XCTAssertEqual(json["user_id"] as? String, "u1")
  }

  func testPagesTolerateNullLists() throws {
    let users = try WarpnetJSON.decode(UsersResponse.self, from: #"{"users":null,"cursor":"end"}"#)
    XCTAssertTrue(users.users.isEmpty)
    XCTAssertEqual(users.cursor, "end")
    let tweets = try WarpnetJSON.decode(TweetsResponse.self, from: "{}")
    XCTAssertTrue(tweets.tweets.isEmpty)
  }

  func testCountersReadTheWireNames() throws {
    let reactions = try WarpnetJSON.decode(ReactionsCountResponse.self, from: #"{"count":3,"reactions":{"❤️":2,"🔥":1}}"#)
    XCTAssertEqual(reactions.reactionsCount, 3)
    XCTAssertEqual(reactions.reactions["🔥"], 1)
    let stats = try WarpnetJSON.decode(TweetStatsResponse.self, from: #"{"tweet_id":"t","reactions_count":5,"replies_count":2,"views_count":9,"my_reaction":"❤️"}"#)
    XCTAssertEqual(stats.reactionsCount, 5)
    XCTAssertEqual(stats.repliesCount, 2)
    XCTAssertEqual(stats.viewsCount, 9)
    XCTAssertEqual(stats.retweetsCount, 0)
    XCTAssertEqual(stats.myReaction, "❤️")
  }

  func testUserDecodesTheOfflineFlag() throws {
    let user = try WarpnetJSON.decode(WarpnetUser.self, from: #"{"id":"u","username":"bob","isOffline":true,"followers_count":7,"locked":true}"#)
    XCTAssertTrue(user.isOffline)
    XCTAssertTrue(user.locked)
    XCTAssertEqual(user.followersCount, 7)
    XCTAssertNil(user.avatarKey)
  }

  func testFollowEventUsesTheNodeFieldNames() throws {
    let json = try WarpnetJSON.encode(NewFollowEvent(followerId: "a", followeeId: "b"))
    let object = try XCTUnwrap(JSONSerialization.jsonObject(with: Data(json.utf8)) as? [String: String])
    XCTAssertEqual(object, ["follower_id": "a", "following_id": "b"])
  }

  func testEncodingKeepsSlashesReadable() throws {
    let json = try WarpnetJSON.encode(GetUserEvent(userId: "ap:https://x.social/users/a"))
    XCTAssertTrue(json.contains("https://x.social/users/a"))
  }

  func testUploadKeysSkipBlanks() throws {
    let response = try WarpnetJSON.decode(UploadImageResponse.self, from: #"{"key1":"k1","key2":"","key3":"k3"}"#)
    XCTAssertEqual(response.keys, ["k1", "k3"])
  }

  func testDecodeFailureIsASerializationError() {
    XCTAssertThrowsError(try WarpnetJSON.decode(WarpnetUser.self, from: "not json")) {
      guard case .serializationError = $0 as? WarpnetError else { return XCTFail("got \($0)") }
    }
  }
}
