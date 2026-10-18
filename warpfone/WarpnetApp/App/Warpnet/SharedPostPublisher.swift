import Env
import Foundation
import Models
import Network
import UserNotifications
import WarpnetTransport

@MainActor
enum SharedPostPublisher {
  private static var isPublishing = false

  static func publish(with client: Client) async {
    guard client.isAuth, !isPublishing else { return }
    isPublishing = true
    defer { isPublishing = false }
    for post in SharedStore.pending() {
      let mediaIds: [String]
      do {
        mediaIds = try await upload(post, with: client)
      } catch {
        guard isRefusal(error) else { return }
        drop(post, title: "warpnet.share.not-published")
        continue
      }
      do {
        let status: Status = try await client.post(endpoint: Statuses.postStatus(json: StatusData(status: post.content,
                                                                                                    mediaIds: mediaIds)))
        StreamWatcher.shared.emmitPostEvent(for: status)
        SharedStore.remove(post)
      } catch {
        if isRefusal(error) {
          drop(post, title: "warpnet.share.not-published")
        } else if mayHaveBeenPosted(error) {
          drop(post, title: "warpnet.share.unconfirmed")
        } else {
          return
        }
      }
    }
  }

  private static func upload(_ post: SharedPost, with client: Client) async throws -> [String] {
    var mediaIds: [String] = []
    for file in post.mediaFiles {
      let data = try Data(contentsOf: SharedStore.directory.appending(path: file))
      let attachment: MediaAttachment = try await client.mediaUpload(endpoint: Media.medias,
                                                                     mimeType: mimeType(of: file),
                                                                     filename: file,
                                                                     data: data)
      mediaIds.append(attachment.id)
    }
    return mediaIds
  }

  private static func drop(_ post: SharedPost, title: String.LocalizationValue) {
    SharedStore.remove(post)
    let content = UNMutableNotificationContent()
    content.title = String(localized: title)
    content.body = post.content
    UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: post.id.uuidString,
                                                                 content: content,
                                                                 trigger: nil))
  }

  // The node answers 5000 for every handler error; its middleware errors are
  // about the request, not the tweet, and pass once the device re-pairs.
  private static func isRefusal(_ error: Error) -> Bool {
    switch error {
    case is CocoaError, is WarpnetRoutingError:
      return true
    case let error as WarpnetError:
      switch error {
      case let .protocolError(_, message):
        return !error.isRateLimited && !message.hasPrefix("middleware:")
      case .serializationError:
        return true
      default:
        return false
      }
    default:
      return false
    }
  }

  // The request went out but its answer was lost, so the node may already
  // have posted it; sending it again would post it twice.
  private static func mayHaveBeenPosted(_ error: Error) -> Bool {
    guard case let WarpnetError.transportFailure(message) = error else { return false }
    return message.lowercased().contains("stream: reading response")
  }

  private static func mimeType(of file: String) -> String {
    switch URL(fileURLWithPath: file).pathExtension.lowercased() {
    case "gif": "image/gif"
    case "mp4", "m4v": "video/mp4"
    case "mov": "video/quicktime"
    default: "image/jpeg"
    }
  }
}
