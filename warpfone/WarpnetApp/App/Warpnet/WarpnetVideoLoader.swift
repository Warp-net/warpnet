import AVFoundation
import Foundation
import MediaUI
import Network
import UniformTypeIdentifiers

final class WarpnetVideoLoader: NSObject, AVAssetResourceLoaderDelegate, @unchecked Sendable {
  static let shared = WarpnetVideoLoader()

  private static let filePrefix = "warpnet-video-"
  private static let keptFiles = 4
  private static let chunkSize = 1 << 20

  private let queue = DispatchQueue(label: "site.warpnet.warpfone.video")
  private let lock = NSLock()
  private var client: Client?
  private var downloads: [URL: Task<URL, Error>] = [:]
  private var order: [URL] = []

  @MainActor
  static func install() {
    shared.removeFiles()
    MediaUIAttachmentVideoViewModel.makePlayerItem = { Self.shared.playerItem(for: $0) }
  }

  func setClient(_ client: Client?) {
    lock.withLock {
      self.client = client
      downloads.values.forEach { $0.cancel() }
      downloads = [:]
      order = []
    }
  }

  func playerItem(for url: URL) -> AVPlayerItem {
    guard url.scheme == "warpnet" else { return AVPlayerItem(url: url) }
    let asset = AVURLAsset(url: url)
    asset.resourceLoader.setDelegate(self, queue: queue)
    return AVPlayerItem(asset: asset)
  }

  func resourceLoader(_: AVAssetResourceLoader,
                      shouldWaitForLoadingOfRequestedResource loadingRequest: AVAssetResourceLoadingRequest) -> Bool
  {
    guard let url = loadingRequest.request.url, url.scheme == "warpnet" else { return false }
    let request = PendingRequest(value: loadingRequest)
    let task = download(url)
    Task { [queue] in
      do {
        let file = try await task.value
        queue.async { Self.respond(to: request.value, from: file) }
      } catch {
        self.forget(url)
        queue.async { request.value.finishLoading(with: error) }
      }
    }
    return true
  }

  private func download(_ url: URL) -> Task<URL, Error> {
    lock.withLock {
      if let task = downloads[url] { return task }
      let client = client
      let task = Task<URL, Error> {
        guard let client else { throw URLError(.notConnectedToInternet) }
        let data = try await client.mediaData(for: url)
        let file = FileManager.default.temporaryDirectory.appendingPathComponent(Self.filePrefix + UUID().uuidString)
        try data.write(to: file, options: .atomic)
        return file
      }
      downloads[url] = task
      order.append(url)
      while order.count > Self.keptFiles {
        let evicted = order.removeFirst()
        if let old = downloads.removeValue(forKey: evicted) {
          Task {
            if let file = try? await old.value {
              try? FileManager.default.removeItem(at: file)
            }
          }
        }
      }
      return task
    }
  }

  private func forget(_ url: URL) {
    lock.withLock {
      downloads[url] = nil
      order.removeAll { $0 == url }
    }
  }

  private func removeFiles() {
    let directory = FileManager.default.temporaryDirectory
    let files = (try? FileManager.default.contentsOfDirectory(at: directory, includingPropertiesForKeys: nil)) ?? []
    for file in files where file.lastPathComponent.hasPrefix(Self.filePrefix) {
      try? FileManager.default.removeItem(at: file)
    }
  }

  private static func respond(to request: AVAssetResourceLoadingRequest, from file: URL) {
    guard !request.isCancelled else { return }
    do {
      let handle = try FileHandle(forReadingFrom: file)
      defer { try? handle.close() }
      let length = try handle.seekToEnd()
      if let info = request.contentInformationRequest {
        try handle.seek(toOffset: 0)
        info.contentType = contentType(try handle.read(upToCount: 12) ?? Data())
        info.contentLength = Int64(length)
        info.isByteRangeAccessSupported = true
      }
      if let dataRequest = request.dataRequest {
        let end = dataRequest.requestsAllDataToEndOfResource
          ? length
          : min(length, UInt64(dataRequest.requestedOffset) + UInt64(dataRequest.requestedLength))
        var offset = UInt64(dataRequest.currentOffset)
        try handle.seek(toOffset: min(offset, length))
        while offset < end, !request.isCancelled {
          guard let chunk = try handle.read(upToCount: Int(min(end - offset, UInt64(chunkSize)))), !chunk.isEmpty else { break }
          dataRequest.respond(with: chunk)
          offset += UInt64(chunk.count)
        }
      }
      guard !request.isCancelled else { return }
      request.finishLoading()
    } catch {
      request.finishLoading(with: error)
    }
  }

  private static func contentType(_ header: Data) -> String {
    let bytes = [UInt8](header)
    if bytes.count >= 12, bytes[4 ..< 8] == [0x66, 0x74, 0x79, 0x70], bytes[8 ..< 12] == [0x71, 0x74, 0x20, 0x20] {
      return UTType.quickTimeMovie.identifier
    }
    return UTType.mpeg4Movie.identifier
  }
}

private struct PendingRequest: @unchecked Sendable {
  let value: AVAssetResourceLoadingRequest
}
