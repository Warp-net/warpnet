import Foundation
import SwiftData

@Model public class SharedPost {
  public var content: String = ""
  public var mediaFiles: [String] = []
  public var creationDate: Date = Date()

  public init(content: String, mediaFiles: [String]) {
    self.content = content
    self.mediaFiles = mediaFiles
    creationDate = Date()
  }
}

public enum SharedStore {
  public static let container: ModelContainer = makeContainer()

  public static var mediaDirectory: URL {
    let root = AppInfo.appGroup.flatMap { FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: $0) }
      ?? URL.applicationSupportDirectory
    return root.appending(path: "SharedPosts", directoryHint: .isDirectory)
  }

  private static func makeContainer() -> ModelContainer {
    let schema = Schema([Draft.self, SharedPost.self])
    if let group = AppInfo.appGroup,
       let container = try? ModelContainer(for: schema,
                                           configurations: ModelConfiguration(schema: schema, groupContainer: .identifier(group)))
    {
      return container
    }
    if let container = try? ModelContainer(for: schema) {
      return container
    }
    return try! ModelContainer(for: schema, configurations: ModelConfiguration(schema: schema, isStoredInMemoryOnly: true))
  }
}
