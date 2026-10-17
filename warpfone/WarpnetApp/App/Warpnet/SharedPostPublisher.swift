import Env
import Foundation
import Models
import Network
import SwiftData
import WarpnetTransport

@MainActor
enum SharedPostPublisher {
  private static var isPublishing = false

  static func publish(with client: Client) async {
    guard client.isAuth, !isPublishing else { return }
    isPublishing = true
    defer { isPublishing = false }
    let context = SharedStore.container.mainContext
    let posts = (try? context.fetch(FetchDescriptor<SharedPost>(sortBy: [SortDescriptor(\.creationDate)]))) ?? []
    for post in posts {
      do {
        var mediaIds: [String] = []
        for file in post.mediaFiles {
          let data = try Data(contentsOf: SharedStore.mediaDirectory.appending(path: file))
          let attachment: MediaAttachment = try await client.mediaUpload(endpoint: Media.medias,
                                                                         version: .v2,
                                                                         method: "POST",
                                                                         mimeType: mimeType(of: file),
                                                                         filename: file,
                                                                         data: data)
          mediaIds.append(attachment.id)
        }
        let status: Status = try await client.post(endpoint: Statuses.postStatus(json: StatusData(status: post.content,
                                                                                                    visibility: .pub,
                                                                                                    mediaIds: mediaIds)))
        StreamWatcher.shared.emmitPostEvent(for: status)
        remove(post, from: context)
      } catch {
        guard isPermanent(error) else { return }
        context.insert(Draft(content: post.content))
        remove(post, from: context)
      }
    }
  }

  private static func remove(_ post: SharedPost, from context: ModelContext) {
    for file in post.mediaFiles {
      try? FileManager.default.removeItem(at: SharedStore.mediaDirectory.appending(path: file))
    }
    context.delete(post)
    try? context.save()
  }

  private static func isPermanent(_ error: Error) -> Bool {
    switch error {
    case is CocoaError, is WarpnetRoutingError:
      return true
    case let error as WarpnetError:
      switch error {
      case .protocolError:
        return !error.isRateLimited
      case .serializationError:
        return true
      default:
        return false
      }
    default:
      return false
    }
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
