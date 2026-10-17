import Foundation

public struct SharedPost: Codable, Sendable {
  public var id = UUID()
  public var content: String
  public var mediaFiles: [String]
  public var creationDate = Date()

  public init(content: String, mediaFiles: [String]) {
    self.content = content
    self.mediaFiles = mediaFiles
  }
}

public enum SharedStore {
  public static var directory: URL {
    let root = AppInfo.appGroup.flatMap { FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: $0) }
      ?? URL.applicationSupportDirectory
    return root.appending(path: "SharedPosts", directoryHint: .isDirectory)
  }

  public static func enqueue(_ post: SharedPost) throws {
    try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    try JSONEncoder().encode(post).write(to: file(of: post), options: .atomic)
  }

  public static func pending() -> [SharedPost] {
    let files = (try? FileManager.default.contentsOfDirectory(at: directory, includingPropertiesForKeys: nil)) ?? []
    return files
      .filter { $0.pathExtension == "json" }
      .compactMap { try? JSONDecoder().decode(SharedPost.self, from: Data(contentsOf: $0)) }
      .sorted { $0.creationDate < $1.creationDate }
  }

  public static func remove(_ post: SharedPost) {
    for name in post.mediaFiles {
      try? FileManager.default.removeItem(at: directory.appending(path: name))
    }
    try? FileManager.default.removeItem(at: file(of: post))
  }

  private static func file(of post: SharedPost) -> URL {
    directory.appending(path: "\(post.id.uuidString).json")
  }
}
