import Foundation
import Models
@testable import Network
import WarpnetTransport
import XCTest

final class WarpnetRouterTests: XCTestCase {
  private let decoder: JSONDecoder = {
    let decoder = JSONDecoder()
    decoder.keyDecodingStrategy = .convertFromSnakeCase
    return decoder
  }()

  private func makeRouter(_ handler: @escaping ScriptedBinding.Handler) async -> (WarpnetRouter, ScriptedBinding) {
    let binding = ScriptedBinding(handler: handler)
    let session = await ScriptedNode.pairedSession(binding)
    return (WarpnetRouter(session: session), binding)
  }

  private static func standardNode(_ route: String, _ body: [String: Any], timeline: [[String: Any]], cursor: String = "end") -> String {
    switch route {
    case ProtocolIds.PUBLIC_GET_USER:
      let id = body["user_id"] as? String ?? ""
      return ScriptedNode.json(ScriptedNode.user(id, "name-\(id)"))
    case ProtocolIds.PUBLIC_GET_TWEET_STATS:
      return ScriptedNode.json(["tweet_id": body["tweet_id"] ?? "", "reactions_count": 1, "replies_count": 0])
    case ProtocolIds.PRIVATE_GET_TIMELINE, ProtocolIds.PUBLIC_GET_TWEETS:
      return ScriptedNode.json(["tweets": timeline, "cursor": cursor])
    default:
      return #"{"code":0,"message":"Accepted"}"#
    }
  }

  func testHomeTimelineDecodesAsStatuses() async throws {
    let timeline = [
      ScriptedNode.tweet("03", by: "A"),
      ScriptedNode.tweet("02", by: "B", extra: ["retweeted_by": "C", "id": "02"]),
      ScriptedNode.tweet("01", by: "A", extra: ["image_keys": ["img1"]]),
    ]
    let (router, binding) = await makeRouter { Self.standardNode($0, $1, timeline: timeline) }
    let response = try await router.route(Timelines.home(sinceId: nil, maxId: nil, minId: nil), method: .get)
    let statuses = try decoder.decode([Status].self, from: response.body)

    XCTAssertEqual(statuses.map(\.id), ["03|A", "02|B||C", "01|A"])
    XCTAssertEqual(statuses[1].account.id, "C")
    XCTAssertEqual(statuses[1].reblog?.account.id, "B")
    XCTAssertEqual(statuses[2].mediaAttachments.first?.url?.absoluteString, "warpnet://image/A/img1")
    XCTAssertEqual(statuses[0].favouritesCount, 1)
    XCTAssertEqual(binding.calls(to: ProtocolIds.PRIVATE_GET_TIMELINE).first?["user_id"] as? String, ScriptedNode.viewerId)
    XCTAssertEqual(Set(binding.calls(to: ProtocolIds.PUBLIC_GET_USER).compactMap { $0["user_id"] as? String }), ["A", "B", "C"])
  }

  func testNextPageUsesTheCursorOfTheLastStatus() async throws {
    let pages: [String: (tweets: [[String: Any]], cursor: String)] = [
      "": ((0 ..< 20).map { ScriptedNode.tweet(String(format: "%02d", 99 - $0), by: "A") }, "cursor-2"),
      "cursor-2": ([ScriptedNode.tweet("10", by: "A")], "end"),
    ]
    let (router, binding) = await makeRouter { route, body in
      if route == ProtocolIds.PRIVATE_GET_TIMELINE {
        let page = pages[body["cursor"] as? String ?? ""] ?? ([], "end")
        return ScriptedNode.json(["tweets": page.tweets, "cursor": page.cursor])
      }
      return Self.standardNode(route, body, timeline: [])
    }
    let first = try await decoder.decode([Status].self,
                                   from: router.route(Timelines.home(sinceId: nil, maxId: nil, minId: nil), method: .get).body)
    XCTAssertEqual(first.count, 20)
    let second = try await decoder.decode([Status].self,
                                    from: router.route(Timelines.home(sinceId: nil, maxId: first.last?.id, minId: nil), method: .get).body)
    XCTAssertEqual(second.map(\.id), ["10|A"])
    XCTAssertEqual(binding.calls(to: ProtocolIds.PRIVATE_GET_TIMELINE).last?["cursor"] as? String, "cursor-2")
    let third = try await decoder.decode([Status].self,
                                   from: router.route(Timelines.home(sinceId: nil, maxId: "10|A", minId: nil), method: .get).body)
    XCTAssertTrue(third.isEmpty)
  }

  func testUnknownMaxIdResumesByWalkingFromTheTop() async throws {
    let pages: [String: (tweets: [[String: Any]], cursor: String)] = [
      "": ([ScriptedNode.tweet("05", by: "A"), ScriptedNode.tweet("04", by: "A")], "c2"),
      "c2": ([ScriptedNode.tweet("03", by: "A"), ScriptedNode.tweet("02", by: "A")], "end"),
    ]
    let (router, _) = await makeRouter { route, body in
      if route == ProtocolIds.PRIVATE_GET_TIMELINE {
        let page = pages[body["cursor"] as? String ?? ""] ?? ([], "end")
        return ScriptedNode.json(["tweets": page.tweets, "cursor": page.cursor])
      }
      return Self.standardNode(route, body, timeline: [])
    }
    let statuses = try await decoder.decode([Status].self,
                                      from: router.route(Timelines.home(sinceId: nil, maxId: "03|A", minId: nil), method: .get).body)
    XCTAssertEqual(statuses.map(\.id), ["02|A"])
  }

  func testNewerStatusesComeFromTheFirstPage() async throws {
    let timeline = [ScriptedNode.tweet("09", by: "A"), ScriptedNode.tweet("08", by: "A"), ScriptedNode.tweet("07", by: "A")]
    let (router, _) = await makeRouter { Self.standardNode($0, $1, timeline: timeline) }
    let newer = try await decoder.decode([Status].self,
                                   from: router.route(Timelines.home(sinceId: nil, maxId: nil, minId: "08|A"), method: .get).body)
    XCTAssertEqual(newer.map(\.id), ["09|A"])
    let none = try await decoder.decode([Status].self,
                                  from: router.route(Timelines.home(sinceId: nil, maxId: nil, minId: "09|A"), method: .get).body)
    XCTAssertTrue(none.isEmpty)
  }

  func testReplyGoesToTheParentAuthor() async throws {
    let (router, binding) = await makeRouter { route, body in
      if route == ProtocolIds.PRIVATE_POST_TWEET {
        var tweet = body
        tweet["id"] = "01NEW"
        tweet["created_at"] = "2026-10-17T10:00:00Z"
        return ScriptedNode.json(tweet)
      }
      return Self.standardNode(route, body, timeline: [])
    }
    let data = StatusData(status: "thanks", visibility: .pub, inReplyToId: "01PARENT|01PAUTHOR|01ROOT", mediaIds: ["img:k1", "vid:v1"])
    let status = try await decoder.decode(Status.self, from: router.route(Statuses.postStatus(json: data), method: .post).body)

    let sent = try XCTUnwrap(binding.calls(to: ProtocolIds.PRIVATE_POST_TWEET).first)
    XCTAssertEqual(sent["parent_id"] as? String, "01PARENT")
    XCTAssertEqual(sent["parent_user_id"] as? String, "01PAUTHOR")
    XCTAssertEqual(sent["root_id"] as? String, "01ROOT")
    XCTAssertEqual(sent["user_id"] as? String, ScriptedNode.viewerId)
    XCTAssertEqual(sent["image_keys"] as? [String], ["k1"])
    XCTAssertEqual(sent["video_key"] as? String, "v1")
    XCTAssertNil(sent["created_at"])
    XCTAssertEqual(status.id, "01NEW|\(ScriptedNode.viewerId)|01ROOT")
    XCTAssertEqual(status.inReplyToId, "01PARENT|01PAUTHOR|01ROOT")
  }

  func testQuoteGoesOutAsARetweetOfTheSource() async throws {
    let (router, binding) = await makeRouter { route, body in
      if route == ProtocolIds.PUBLIC_POST_RETWEET {
        var tweet = body
        tweet["id"] = "01QUOTE"
        tweet["created_at"] = "2026-10-17T10:00:00Z"
        return ScriptedNode.json(tweet)
      }
      return Self.standardNode(route, body, timeline: [])
    }
    let data = StatusData(status: " look at this ", visibility: .pub, quotedStatusId: "01SRC|01SRCAUTHOR||01RT")
    let status = try await decoder.decode(Status.self, from: router.route(Statuses.postStatus(json: data), method: .post).body)

    let sent = try XCTUnwrap(binding.calls(to: ProtocolIds.PUBLIC_POST_RETWEET).first)
    XCTAssertEqual(sent["id"] as? String, "01SRC")
    XCTAssertEqual(sent["quoted_tweet_id"] as? String, "01SRC")
    XCTAssertEqual(sent["quoted_user_id"] as? String, "01SRCAUTHOR")
    XCTAssertEqual(sent["retweeted_by"] as? String, ScriptedNode.viewerId)
    XCTAssertEqual(sent["user_id"] as? String, ScriptedNode.viewerId)
    XCTAssertEqual(sent["text"] as? String, "look at this")
    XCTAssertTrue(binding.calls(to: ProtocolIds.PRIVATE_POST_TWEET).isEmpty)
    XCTAssertEqual(status.id, "01QUOTE|\(ScriptedNode.viewerId)")
    XCTAssertNil(status.reblog)
    XCTAssertEqual(status.content.statusesURLs.map(\.absoluteString), ["warpnet://status/01SRC%7C01SRCAUTHOR"])
  }

  func testFavoriteReactsWithTheDefaultHeart() async throws {
    let (router, binding) = await makeRouter { route, body in
      switch route {
      case ProtocolIds.PUBLIC_POST_REACT:
        return #"{"count":8,"reactions":{"❤️":8}}"#
      case ProtocolIds.PUBLIC_GET_TWEET:
        return ScriptedNode.json(ScriptedNode.tweet("01T", by: "01A", createdAt: "2020-01-01T00:00:00Z"))
      default:
        return Self.standardNode(route, body, timeline: [])
      }
    }
    let status = try await decoder.decode(Status.self, from: router.route(Statuses.favorite(id: "01T|01A"), method: .post).body)
    let react = try XCTUnwrap(binding.calls(to: ProtocolIds.PUBLIC_POST_REACT).first)
    XCTAssertEqual(react["tweet_id"] as? String, "01T")
    XCTAssertEqual(react["user_id"] as? String, "01A")
    XCTAssertEqual(react["owner_id"] as? String, ScriptedNode.viewerId)
    XCTAssertEqual(status.id, "01T|01A")
    XCTAssertGreaterThan(status.favouritesCount, 0)
  }

  func testReactionCarriesTheChosenEmoji() async throws {
    let (router, binding) = await makeRouter { route, body in
      switch route {
      case ProtocolIds.PUBLIC_POST_REACT:
        return #"{"count":3,"reactions":{"🔥":1,"❤️":2}}"#
      case ProtocolIds.PUBLIC_GET_TWEET:
        return ScriptedNode.json(ScriptedNode.tweet("01T", by: "01A", createdAt: "2020-01-01T00:00:00Z"))
      case ProtocolIds.PUBLIC_GET_TWEET_STATS:
        return #"{"tweet_id":"01T","reactions_count":3,"my_reaction":"🔥"}"#
      default:
        return Self.standardNode(route, body, timeline: [])
      }
    }
    let status = try await decoder.decode(Status.self, from: router.route(Statuses.react(id: "01T|01A", emoji: "🔥"), method: .post).body)
    XCTAssertEqual(binding.calls(to: ProtocolIds.PUBLIC_POST_REACT).first?["emoji"] as? String, "🔥")
    XCTAssertEqual(status.myReaction, "🔥")
    XCTAssertEqual(status.favourited, true)
  }

  func testRelationshipProbesBothDirections() async throws {
    let (router, _) = await makeRouter { route, body in
      switch route {
      case ProtocolIds.PUBLIC_POST_IS_FOLLOWING: return #"{"is_following":true}"#
      case ProtocolIds.PUBLIC_POST_IS_FOLLOWER: return #"{"is_follower":false}"#
      default: return Self.standardNode(route, body, timeline: [])
      }
    }
    let relationships = try await decoder.decode([Relationship].self,
                                           from: router.route(Accounts.relationships(ids: ["01X"]), method: .get).body)
    XCTAssertEqual(relationships.first?.id, "01X")
    XCTAssertEqual(relationships.first?.following, true)
    XCTAssertEqual(relationships.first?.followedBy, false)
  }

  func testThreadContextWalksUpAndDown() async throws {
    let (router, binding) = await makeRouter { route, body in
      switch route {
      case ProtocolIds.PUBLIC_GET_TWEET:
        switch body["tweet_id"] as? String {
        case "01REPLY":
          return ScriptedNode.json(ScriptedNode.tweet("01REPLY", by: "01B", extra: ["parent_id": "01ROOT", "parent_user_id": "01A", "root_id": "01ROOT"]))
        default:
          return ScriptedNode.json(ScriptedNode.tweet("01ROOT", by: "01A"))
        }
      case ProtocolIds.PUBLIC_GET_TWEETS:
        if body["parent_id"] as? String == "01REPLY" {
          return ScriptedNode.json(["tweets": [ScriptedNode.tweet("01CHILD", by: "01C", extra: ["parent_id": "01REPLY", "parent_user_id": "01B", "root_id": "01ROOT"])], "cursor": "end"])
        }
        return ScriptedNode.json(["tweets": [], "cursor": "end"])
      default:
        return Self.standardNode(route, body, timeline: [])
      }
    }
    let context = try await decoder.decode(StatusContext.self, from: router.route(Statuses.context(id: "01REPLY|01B|01ROOT"), method: .get).body)
    XCTAssertEqual(context.ancestors.map(\.id), ["01ROOT|01A"])
    XCTAssertEqual(context.descendants.map(\.id), ["01CHILD|01C|01ROOT"])
    XCTAssertEqual(context.descendants.first?.inReplyToId, "01REPLY|01B|01ROOT")
    let replies = try XCTUnwrap(binding.calls(to: ProtocolIds.PUBLIC_GET_TWEETS).first)
    XCTAssertEqual(replies["root_id"] as? String, "01ROOT")
    XCTAssertEqual(replies["root_user_id"] as? String, "01B")
  }

  func testUnsupportedEndpointsAreTypedErrors() async {
    let (router, _) = await makeRouter { Self.standardNode($0, $1, timeline: []) }
    do {
      _ = try await router.route(Trends.tags, method: .get)
      XCTFail("expected a failure")
    } catch {
      XCTAssertEqual(error as? WarpnetRoutingError, .unsupported("trends/tags"))
    }
  }

  func testUploadTagsTheMediaKind() async throws {
    let (router, binding) = await makeRouter { route, body in
      switch route {
      case ProtocolIds.PRIVATE_POST_UPLOAD_IMAGE: return #"{"key1":"hash1"}"#
      default: return Self.standardNode(route, body, timeline: [])
      }
    }
    let body = try await router.upload(Media.medias, mimeType: "image/jpeg", filename: "file", data: Data([1, 2, 3]))
    let attachment = try decoder.decode(MediaAttachment.self, from: body)
    XCTAssertEqual(attachment.id, "img:hash1")
    XCTAssertEqual(attachment.url?.absoluteString, "warpnet://image/\(ScriptedNode.viewerId)/hash1")
    XCTAssertEqual(binding.calls(to: ProtocolIds.PRIVATE_POST_UPLOAD_IMAGE).first?["image1"] as? String, "image/jpeg,AQID")
  }

  func testNotificationsResolveTheActor() async throws {
    let (router, _) = await makeRouter { route, body in
      switch route {
      case ProtocolIds.PRIVATE_GET_NOTIFICATIONS:
        return ScriptedNode.json(["notifications": [
          ["id": "02", "type": "follow", "text": "Bob followed you", "actor_id": "01BOB", "created_at": "2026-10-17T08:00:00Z"],
          ["id": "01", "type": "new_user", "text": "x", "created_at": "2026-10-17T07:00:00Z"],
        ], "cursor": "end"])
      default:
        return Self.standardNode(route, body, timeline: [])
      }
    }
    let notifications = try await decoder.decode([Models.Notification].self,
                                           from: router.route(Notifications.notifications(minId: nil, maxId: nil, types: nil, limit: 40), method: .get).body)
    XCTAssertEqual(notifications.map(\.id), ["02"])
    XCTAssertEqual(notifications.first?.supportedType, .follow)
    XCTAssertEqual(notifications.first?.account.id, "01BOB")
  }

  func testQuoteURLResolvesToTheQuotedStatus() async throws {
    let (router, binding) = await makeRouter { route, body in
      switch route {
      case ProtocolIds.PUBLIC_GET_TWEET:
        return ScriptedNode.json(ScriptedNode.tweet("01SRC", by: "01SRCAUTHOR"))
      default:
        return Self.standardNode(route, body, timeline: [])
      }
    }
    let results = try await decoder.decode(SearchResults.self,
                                           from: router.route(Search.search(query: "warpnet://status/01SRC%7C01SRCAUTHOR",
                                                                            type: "statuses", offset: 0, following: nil),
                                                              method: .get).body)
    XCTAssertEqual(results.statuses.map(\.id), ["01SRC|01SRCAUTHOR"])
    XCTAssertEqual(binding.calls(to: ProtocolIds.PUBLIC_GET_TWEET).first?["user_id"] as? String, "01SRCAUTHOR")
  }

  func testReportsNameTheModerationClasses() async throws {
    XCTAssertEqual(WarpnetReportCategory.reason(for: [.hate, .spam, .harassment], details: "  "),
                   "Non-Violent Crimes, Hate")
    XCTAssertEqual(WarpnetReportCategory.reason(for: [.privacy], details: " doxxing "), "Privacy — doxxing")

    let (router, binding) = await makeRouter { Self.standardNode($0, $1, timeline: []) }
    _ = try await router.route(Statuses.report(accountId: "01A", statusId: "01T|01A|01ROOT", comment: "Hate"), method: .post)
    _ = try await router.route(Statuses.report(accountId: "01B", statusId: nil,
                                               comment: "Privacy — " + String(repeating: "é", count: 300)),
                               method: .post)

    let reports = binding.calls(to: ProtocolIds.PUBLIC_POST_REPORT)
    XCTAssertEqual(reports.count, 2)
    XCTAssertEqual(reports[0]["type"] as? Int, 1)
    XCTAssertEqual(reports[0]["object_id"] as? String, "01T")
    XCTAssertEqual(reports[0]["target_user_id"] as? String, "01A")
    XCTAssertEqual(reports[0]["target_node_id"] as? String, "12D3KooW01A")
    XCTAssertEqual(reports[1]["type"] as? Int, 0)
    XCTAssertEqual(reports[1]["object_id"] as? String, "")
    XCTAssertEqual((reports[1]["reason"] as? String)?.unicodeScalars.count, WarpnetLimits.maxReportReasonRunes)
  }

  func testChatsBecomeConversationsWithTheOtherParty() async throws {
    let viewer = ScriptedNode.viewerId
    let (router, binding) = await makeRouter { route, body in
      switch route {
      case ProtocolIds.PRIVATE_GET_CHATS:
        return ScriptedNode.json(["chats": [
          ["id": "chat1", "owner_id": viewer, "other_user_id": "01BOB", "last_message": "see you", "updated_at": "2026-10-17T08:00:00Z"],
          ["id": "chat2", "owner_id": "01CAROL", "other_user_id": viewer, "last_message": "", "created_at": "2026-10-16T08:00:00Z"],
        ], "cursor": "end"])
      case ProtocolIds.PRIVATE_GET_MESSAGES:
        return ScriptedNode.json(["chat_id": "chat1", "cursor": "end", "messages": [
          ["id": "02", "chat_id": "chat1", "sender_id": viewer, "receiver_id": "01BOB", "text": "see you", "created_at": "2026-10-17T08:00:00Z"],
          ["id": "01", "chat_id": "chat1", "sender_id": "01BOB", "receiver_id": viewer, "text": "hi", "created_at": "2026-10-17T07:59:00Z"],
        ]])
      case ProtocolIds.PUBLIC_POST_MESSAGE:
        var message = body
        message["id"] = "03"
        message["created_at"] = "2026-10-17T08:01:00Z"
        return ScriptedNode.json(message)
      default:
        return Self.standardNode(route, body, timeline: [])
      }
    }
    let conversations = try await decoder.decode([Conversation].self,
                                                 from: router.route(Chats.chats(cursor: nil), method: .get).body)
    XCTAssertEqual(conversations.map(\.id), ["chat1", "chat2"])
    XCTAssertEqual(conversations.map { $0.accounts.first?.id }, ["01BOB", "01CAROL"])
    XCTAssertEqual(conversations[0].lastStatus?.content.asRawText, "see you")
    XCTAssertNil(conversations[1].lastStatus)

    let messages = try await decoder.decode([Status].self,
                                            from: router.route(Chats.messages(chatId: "chat1"), method: .get).body)
    XCTAssertEqual(messages.map(\.id), ["01", "02"])
    XCTAssertEqual(messages.map(\.account.id), ["01BOB", viewer])

    let sent = try await decoder.decode(Status.self,
                                        from: router.route(Chats.send(chatId: "chat1", receiverId: "01BOB", text: "ok"), method: .post).body)
    XCTAssertEqual(sent.id, "03")
    let event = try XCTUnwrap(binding.calls(to: ProtocolIds.PUBLIC_POST_MESSAGE).first)
    XCTAssertEqual(event["sender_id"] as? String, viewer)
    XCTAssertEqual(event["receiver_id"] as? String, "01BOB")
    XCTAssertEqual(event["text"] as? String, "ok")
  }

  func testProfileEditsGoToTheOwnUser() async throws {
    let (router, binding) = await makeRouter { route, body in
      switch route {
      case ProtocolIds.PRIVATE_POST_USER:
        var user = ScriptedNode.user(ScriptedNode.viewerId, body["username"] as? String ?? "Me")
        user["avatar_key"] = body["avatar_key"]
        return ScriptedNode.json(user)
      case ProtocolIds.PRIVATE_POST_UPLOAD_IMAGE:
        return #"{"key1":"newavatar"}"#
      default:
        return Self.standardNode(route, body, timeline: [])
      }
    }
    let data = UpdateCredentialsData(displayName: " Alice ", note: "bio", source: .init(privacy: .pub, sensitive: false),
                                     bot: false, locked: false, discoverable: false,
                                     fieldsAttributes: [.init(name: "Website", value: "https://alice.example ")])
    _ = try await router.route(Accounts.updateCredentials(json: data), method: .patch)
    let edit = try XCTUnwrap(binding.calls(to: ProtocolIds.PRIVATE_POST_USER).first)
    XCTAssertEqual(edit["username"] as? String, "Alice")
    XCTAssertEqual(edit["bio"] as? String, "bio")
    XCTAssertEqual(edit["website"] as? String, "https://alice.example")
    XCTAssertNil(edit["avatar_key"])

    let body = try await router.upload(Accounts.updateCredentialsMedia, mimeType: "image/jpeg", filename: "avatar", data: Data([1]))
    let account = try decoder.decode(Account.self, from: body)
    XCTAssertEqual(binding.calls(to: ProtocolIds.PRIVATE_POST_USER).last?["avatar_key"] as? String, "newavatar")
    XCTAssertNil(binding.calls(to: ProtocolIds.PRIVATE_POST_USER).last?["username"])
    XCTAssertEqual(account.avatar.absoluteString, "warpnet://image/\(ScriptedNode.viewerId)/newavatar")
  }

  func testFediverseHandles() {
    XCTAssertEqual(WarpnetRouter.fediverseHandle("@alice@Mastodon.Social"), "alice@mastodon.social")
    XCTAssertEqual(WarpnetRouter.fediverseHandle("https://mastodon.social/@bob"), "bob@mastodon.social")
    XCTAssertNil(WarpnetRouter.fediverseHandle("alice"))
  }
}
