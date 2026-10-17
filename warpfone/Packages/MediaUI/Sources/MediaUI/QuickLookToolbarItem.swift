// Modified by Warpnet. See NOTICE.

import ImageIO
import SwiftUI
import NukeUI
import Nuke
import UniformTypeIdentifiers

struct QuickLookToolbarItem: ToolbarContent, @unchecked Sendable {
  let itemUrl: URL
  @State private var localPath: URL?
  @State private var isLoading = false

  var body: some ToolbarContent {
    ToolbarItem(placement: .topBarTrailing) {
      Button {
        Task {
          isLoading = true
          localPath = await localPathFor(url: itemUrl)
          isLoading = false
        }
      } label: {
        if isLoading {
          ProgressView()
        } else {
          Image(systemName: "info.circle")
        }
      }
      .quickLookPreview($localPath)
    }
  }

  private func imageData(_ url: URL) async -> Data? {
    var data = ImagePipeline.shared.cache.cachedData(for: .init(url: url))
    if data == nil {
      data = try? await ImagePipeline.shared.data(for: url).0
    }
    return data
  }

  private func localPathFor(url: URL) async -> URL {
    try? FileManager.default.removeItem(at: quickLookDir)
    try? FileManager.default.createDirectory(at: quickLookDir, withIntermediateDirectories: true)
    let data = await imageData(url)
    var name = url.lastPathComponent
    if url.pathExtension.isEmpty, let data, let fileExtension = Self.fileExtension(of: data) {
      name += ".\(fileExtension)"
    }
    let path = quickLookDir.appendingPathComponent(name)
    try? data?.write(to: path)
    return path
  }

  private static func fileExtension(of data: Data) -> String? {
    if let source = CGImageSourceCreateWithData(data as CFData, nil),
       let type = CGImageSourceGetType(source),
       let fileExtension = UTType(type as String)?.preferredFilenameExtension
    {
      return fileExtension
    }
    return data.dropFirst(4).prefix(4) == Data("ftyp".utf8) ? "mp4" : nil
  }

  private var quickLookDir: URL {
    try! FileManager.default.url(for: .cachesDirectory,
                                 in: .userDomainMask,
                                 appropriateFor: nil,
                                 create: false)
      .appending(component: "quicklook")
  }
}
