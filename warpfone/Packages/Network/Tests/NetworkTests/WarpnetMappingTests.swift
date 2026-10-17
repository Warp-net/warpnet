import Foundation
import Models
@testable import Network
import WarpnetTransport
import XCTest

final class WarpnetMappingTests: XCTestCase {
  private func decode<T: Decodable>(_ type: T.Type, _ value: some Encodable) throws -> T {
    let decoder = JSONDecoder()
    decoder.keyDecodingStrategy = .convertFromSnakeCase
    return try decoder.decode(type, from: MastodonJSON.encode(value))
  }

  private let author = WarpnetUser(id: "01AUTHOR",
                                   username: "Alice",
                                   bio: "I <3 p2p",
                                   avatarKey: "avatarhash",
                                   createdAt: "2025-01-02T03:04:05Z",
                                   followingsCount: 2,
                                   followersCount: 7,
                                   tweetsCount: 11,
                                   website: "https://alice.example",
                                   locked: true)

  func testStatusDecodesIntoTheMastodonModel() throws {
    let tweet = WarpnetTweet(id: "01TWEET",
                             createdAt: "2026-10-17T08:29:00.123456789Z",
                             rootId: "01TWEET",
                             text: "Hi <b>there</b>\nsee https://example.com/a?b=c.",
                             userId: "01AUTHOR",
                             username: "Alice",
                             imageKeys: ["k1", "k2"],
                             pinned: true)
    let stats = TweetStatsResponse(tweetId: "01TWEET", retweetsCount: 2, reactionsCount: 5, repliesCount: 3, myReaction: "❤️")
    let mapped = WarpnetMapper.status(tweet, author: author, viewer: ViewerState(stats: stats, isBookmarked: true))
    let status = try decode(Status.self, mapped)

    XCTAssertEqual(status.id, "01TWEET|01AUTHOR")
    XCTAssertEqual(status.content.asRawText.escape(), "Hi <b>there</b>\nsee https://example.com/a?b=c.")
    XCTAssertEqual(status.content.links.first?.url.absoluteString, "https://example.com/a?b=c")
    XCTAssertEqual(status.createdAt.asDate.timeIntervalSince1970, 1_792_225_740.123, accuracy: 0.001)
    XCTAssertEqual(status.account.displayName, "Alice")
    XCTAssertEqual(status.account.acct, "01AUTHOR")
    XCTAssertEqual(status.account.avatar.absoluteString, "warpnet://image/01AUTHOR/avatarhash")
    XCTAssertTrue(status.account.haveAvatar)
    XCTAssertFalse(status.account.haveHeader)
    XCTAssertTrue(status.account.locked)
    XCTAssertEqual(status.account.fields.first?.value.asRawText, "https://alice.example")
    XCTAssertEqual(status.account.note.asRawText.escape(), "I <3 p2p")
    XCTAssertEqual(status.mediaAttachments.map(\.url?.absoluteString), ["warpnet://image/01AUTHOR/k1", "warpnet://image/01AUTHOR/k2"])
    XCTAssertEqual(status.mediaAttachments.first?.supportedType, .image)
    XCTAssertEqual(status.repliesCount, 3)
    XCTAssertEqual(status.reblogsCount, 2)
    XCTAssertEqual(status.favouritesCount, 5)
    XCTAssertEqual(status.favourited, true)
    XCTAssertEqual(status.bookmarked, true)
    XCTAssertEqual(status.pinned, true)
    XCTAssertEqual(status.visibility, .pub)
    XCTAssertNil(status.inReplyToId)
    XCTAssertNil(status.editedAt)
  }

  func testReplyNamesItsParent() throws {
    let reply = WarpnetTweet(id: "01REPLY",
                             createdAt: "2026-10-17T09:00:00Z",
                             parentId: "01PARENT",
                             parentUserId: "01PARENTAUTHOR",
                             rootId: "01ROOT",
                             text: "reply",
                             userId: "01AUTHOR",
                             username: "Alice")
    let status = try decode(Status.self, WarpnetMapper.status(reply, author: author, viewer: ViewerState()))
    XCTAssertEqual(status.id, "01REPLY|01AUTHOR|01ROOT")
    XCTAssertEqual(status.inReplyToId, "01PARENT|01PARENTAUTHOR|01ROOT")
    XCTAssertEqual(status.inReplyToAccountId, "01PARENTAUTHOR")

    let direct = WarpnetTweet(id: "01REPLY", parentId: "01ROOT", parentUserId: "01ROOTAUTHOR", rootId: "01ROOT",
                              text: "reply", userId: "01AUTHOR", username: "Alice")
    let directStatus = try decode(Status.self, WarpnetMapper.status(direct, author: author, viewer: ViewerState()))
    XCTAssertEqual(directStatus.inReplyToId, "01ROOT|01ROOTAUTHOR")
  }

  func testRetweetWrapsTheOriginal() throws {
    let retweeter = WarpnetUser(id: "01BOB", username: "Bob")
    let item = WarpnetTweet(id: "RT:01TWEET", retweetedBy: "01BOB", rootId: "01TWEET", text: "original", userId: "01AUTHOR", username: "Alice")
    let original = WarpnetMapper.status(item, author: author, viewer: ViewerState())
    let wrapper = WarpnetMapper.retweet(original,
                                        address: WarpnetMapper.address(item),
                                        retweeter: WarpnetMapper.account(retweeter))
    let status = try decode(Status.self, wrapper)
    XCTAssertEqual(status.id, "01TWEET|01AUTHOR||01BOB")
    XCTAssertEqual(status.account.id, "01BOB")
    XCTAssertEqual(status.reblog?.id, "01TWEET|01AUTHOR")
    XCTAssertEqual(status.reblog?.content.asRawText, "original")
    XCTAssertEqual(status.reblog?.account.id, "01AUTHOR")
  }

  func testPollKeepsVotesAndExpiry() throws {
    let tweet = WarpnetTweet(id: "01POLL", createdAt: "2026-10-17T08:00:00Z", rootId: "01POLL", text: "?",
                             userId: "01AUTHOR", username: "Alice",
                             poll: WarpnetPoll(options: ["yes", "no"], expiresAt: "2099-01-01T00:00:00Z"))
    let results = try WarpnetJSON.decode(PollResultsResponse.self, from: #"{"tweet_id":"01POLL","votes":[3,1],"total_votes":4,"voted_option":1}"#)
    let status = try decode(Status.self, WarpnetMapper.status(tweet, author: author, viewer: ViewerState(poll: results)))
    let poll = try XCTUnwrap(status.poll)
    XCTAssertEqual(poll.id, "01POLL|01AUTHOR")
    XCTAssertEqual(poll.options.map(\.title), ["yes", "no"])
    XCTAssertEqual(poll.options.map(\.votesCount), [3, 1])
    XCTAssertEqual(poll.votesCount, 4)
    XCTAssertEqual(poll.voted, true)
    XCTAssertEqual(poll.ownVotes, [1])
    XCTAssertFalse(poll.expired)
    XCTAssertEqual(try XCTUnwrap(poll.expiresAt.value).asDate.timeIntervalSince1970, 4_070_908_800, accuracy: 1)

    let open = WarpnetTweet(id: "01OPEN", rootId: "01OPEN", text: "?", userId: "01AUTHOR", username: "Alice",
                            poll: WarpnetPoll(options: ["a", "b"]))
    let openPoll = try XCTUnwrap(decode(Status.self, WarpnetMapper.status(open, author: author, viewer: ViewerState())).poll)
    XCTAssertNil(openPoll.expiresAt.value)
  }

  func testEditedTweetCarriesTheEditDate() throws {
    let tweet = WarpnetTweet(id: "01EDIT", createdAt: "2026-10-17T07:00:00Z", updatedAt: "2026-10-17T09:30:00.5+02:00",
                             rootId: "01EDIT", text: "x", userId: "01AUTHOR", username: "Alice")
    let status = try decode(Status.self, WarpnetMapper.status(tweet, author: nil, viewer: ViewerState()))
    XCTAssertEqual(try XCTUnwrap(status.editedAt).asDate.timeIntervalSince1970, 1_792_222_200.5, accuracy: 0.001)
    XCTAssertEqual(status.account.displayName, "Alice")
    XCTAssertFalse(status.account.haveAvatar)
  }

  func testVideoBecomesAVideoAttachment() throws {
    let tweet = WarpnetTweet(id: "01VID", rootId: "01VID", text: "", userId: "01AUTHOR", username: "Alice", videoKey: "v1")
    let status = try decode(Status.self, WarpnetMapper.status(tweet, author: author, viewer: ViewerState()))
    XCTAssertEqual(status.mediaAttachments.first?.supportedType, .video)
    XCTAssertEqual(status.mediaAttachments.first?.url?.absoluteString, "warpnet://video/01AUTHOR/v1")
  }

  func testQuoteBecomesAnEmbed() throws {
    let tweet = WarpnetTweet(id: "01Q", retweetedBy: "01AUTHOR", rootId: "01Q", text: "look", userId: "01AUTHOR", username: "Alice",
                             quotedTweetId: "01SRC", quotedUserId: "01SRCAUTHOR")
    XCTAssertFalse(WarpnetMapper.address(tweet).isRetweet)
    let quote = WarpnetFormat.statusURL(id: "01SRC|01SRCAUTHOR")
    XCTAssertEqual(quote, "warpnet://status/01SRC%7C01SRCAUTHOR")
    let status = try decode(Status.self, WarpnetMapper.status(tweet, author: author, viewer: ViewerState(), quote: quote))
    XCTAssertEqual(status.content.statusesURLs.map(\.absoluteString), [quote])
    XCTAssertEqual(status.content.asRawText, "look")
    XCTAssertEqual(WarpnetFormat.statusId(fromURL: quote), "01SRC|01SRCAUTHOR")

    let escaped = StatusAddress(tweetId: "a|b", userId: "01U").id
    XCTAssertEqual(WarpnetFormat.statusId(fromURL: WarpnetFormat.statusURL(id: escaped)), escaped)
    XCTAssertNil(WarpnetFormat.statusId(fromURL: "warpnet://image/01U/k"))
  }

  func testAccountSourceCarriesTheEditableBio() throws {
    let account = try decode(Account.self, WarpnetMapper.account(author))
    XCTAssertEqual(account.source?.note, "I <3 p2p")
    XCTAssertEqual(account.followersCount, 7)
    XCTAssertEqual(account.followingCount, 2)
    XCTAssertEqual(account.statusesCount, 11)
    XCTAssertEqual(account.createdAt.asDate.timeIntervalSince1970, 1_735_787_045, accuracy: 0.001)
  }

  func testSyntheticInstanceAdvertisesWarpnetLimits() throws {
    let instance = try decode(Instance.self, WarpnetMapper.instance())
    XCTAssertEqual(instance.configuration?.statuses.maxCharacters, 2048)
    XCTAssertEqual(instance.configuration?.statuses.maxMediaAttachments, 4)
    XCTAssertEqual(instance.configuration?.polls.maxOptions, 4)
    XCTAssertEqual(instance.configuration?.polls.maxCharactersPerOption, 25)
    XCTAssertEqual(instance.version, "4.0.0")
  }

  func testRelationshipAndContextDecode() throws {
    let relationship = try decode(Relationship.self, WarpnetMapper.relationship(userId: "01X", following: true, followedBy: false, blocking: true))
    XCTAssertTrue(relationship.following)
    XCTAssertTrue(relationship.blocking)
    XCTAssertTrue(relationship.showingReblogs)
    let tweet = WarpnetTweet(id: "01T", rootId: "01T", text: "x", userId: "01AUTHOR", username: "Alice")
    let status = WarpnetMapper.status(tweet, author: author, viewer: ViewerState())
    let context = try decode(StatusContext.self, MastodonContext(ancestors: [status], descendants: []))
    XCTAssertEqual(context.ancestors.first?.id, "01T|01AUTHOR")
  }

  func testPlainTextBecomesSafeHTML() {
    XCTAssertEqual(WarpnetFormat.html(fromPlainText: "a & b\n\nc\nd"), "<p>a &amp; b</p><p>c<br>d</p>")
    XCTAssertEqual(WarpnetFormat.html(fromPlainText: "(https://x.io/a)"), #"<p>(<a href="https://x.io/a">https://x.io/a</a>)</p>"#)
    XCTAssertEqual(WarpnetFormat.html(fromPlainText: "https://x.io/w_(1)"), #"<p><a href="https://x.io/w_(1)">https://x.io/w_(1)</a></p>"#)
    XCTAssertEqual(WarpnetFormat.html(fromPlainText: #"<script>"x"</script>"#), "<p>&lt;script&gt;&quot;x&quot;&lt;/script&gt;</p>")
    XCTAssertEqual(WarpnetFormat.html(fromPlainText: ""), "")
  }

  func testServerDatesUseMillisecondsAndUTC() {
    XCTAssertEqual(WarpnetFormat.serverDate(fromRFC3339: "2026-10-17T08:29:00.123456789Z"), "2026-10-17T08:29:00.123Z")
    XCTAssertEqual(WarpnetFormat.serverDate(fromRFC3339: "2026-10-17T10:29:00+02:00"), "2026-10-17T08:29:00.000Z")
    XCTAssertEqual(WarpnetFormat.serverDate(fromRFC3339: "garbage"), "1970-01-01T00:00:00.000Z")
  }

  func testStatusAddressRoundTrips() {
    let address = StatusAddress(tweetId: "RT:01T", userId: "ap:https://x.social/users/a|b", rootId: "01R", retweetedBy: "01B", parentId: "01P")
    XCTAssertEqual(address.tweetId, "01T")
    let parsed = StatusAddress(id: address.id)
    XCTAssertEqual(parsed?.tweetId, "01T")
    XCTAssertEqual(parsed?.userId, "ap:https://x.social/users/a|b")
    XCTAssertEqual(parsed?.rootId, "01R")
    XCTAssertEqual(parsed?.retweetedBy, "01B")
    XCTAssertEqual(parsed?.parentId, "")
    XCTAssertNil(StatusAddress(id: "01PLAIN"))
    XCTAssertEqual(StatusAddress(tweetId: "01T", userId: "01U", rootId: "01T").id, "01T|01U")
  }

  func testLinkHandlerKeepsOpaqueCursors() {
    let link = LinkHandler(maxId: "users/01AB:c|d")
    XCTAssertEqual(link.maxId, "users/01AB:c|d")
    XCTAssertEqual(LinkHandler(rawLink: "<https://m.s/api/v1/x?max_id=01HZXY&limit=40>; rel=\"next\"").maxId, "01HZXY")
  }
}
