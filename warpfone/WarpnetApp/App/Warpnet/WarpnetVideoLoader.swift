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
  // A node call can't be cancelled once it runs, so videos scrolled past
  // must be dropped while they still wait for a slot.
  private let slots = DownloadSlots(2)
  private let lock = NSLock()
  private var client: Client?
  private var downloads: [URL: (token: UUID, task: Task<URL, Error>)] = [:]
  private var files: [URL: URL] = [:]
  private var order: [URL] = []
  private var wanted: [URL: Int] = [:]
  private var active: [ObjectIdentifier: URL] = [:]

  @MainActor
  static func install() {
    shared.removeFiles()
    MediaUIAttachmentVideoViewModel.makePlayerItem = { Self.shared.playerItem(for: $0) }
  }

  func setClient(_ client: Client?) {
    lock.withLock {
      self.client = client
      downloads.values.forEach { $0.task.cancel() }
      downloads = [:]
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
    let id = ObjectIdentifier(loadingRequest)
    let task: Task<URL, Error> = lock.withLock {
      active[id] = url
      wanted[url, default: 0] += 1
      if let file = files[url] {
        return Task<URL, Error> { file }
      }
      if let download = downloads[url] {
        return download.task
      }
      let token = UUID()
      let task = download(url, token: token)
      downloads[url] = (token, task)
      return task
    }
    Task { [queue] in
      let result = await task.result
      queue.async {
        switch result {
        case let .success(file):
          Self.respond(to: request.value, from: file)
        case let .failure(error):
          if !request.value.isCancelled {
            request.value.finishLoading(with: error)
          }
        }
        self.release(id)
      }
    }
    return true
  }

  func resourceLoader(_: AVAssetResourceLoader, didCancel loadingRequest: AVAssetResourceLoadingRequest) {
    release(ObjectIdentifier(loadingRequest))
  }

  private func download(_ url: URL, token: UUID) -> Task<URL, Error> {
    let client = client
    return Task<URL, Error> { [slots] in
      guard let client else { throw URLError(.notConnectedToInternet) }
      await slots.acquire()
      do {
        try Task.checkCancellation()
        let data = try await client.mediaData(for: url)
        try Task.checkCancellation()
        let file = FileManager.default.temporaryDirectory.appendingPathComponent(Self.filePrefix + UUID().uuidString)
        try data.write(to: file, options: .atomic)
        await slots.release()
        self.stored(file, for: url, token: token)
        return file
      } catch {
        await slots.release()
        self.failed(url, token: token)
        throw error
      }
    }
  }

  private func stored(_ file: URL, for url: URL, token: UUID) {
    lock.withLock {
      if downloads[url]?.token == token {
        downloads[url] = nil
      }
      files[url] = file
      order.append(url)
      evict()
    }
  }

  private func failed(_ url: URL, token: UUID) {
    lock.withLock {
      if downloads[url]?.token == token {
        downloads[url] = nil
      }
    }
  }

  private func release(_ id: ObjectIdentifier) {
    lock.withLock {
      guard let url = active.removeValue(forKey: id) else { return }
      let remaining = (wanted[url] ?? 1) - 1
      wanted[url] = remaining > 0 ? remaining : nil
      if remaining <= 0, let download = downloads.removeValue(forKey: url) {
        download.task.cancel()
      }
      evict()
    }
  }

  // Called with the lock held. A file that a pending request still reads stays.
  private func evict() {
    var index = 0
    while order.count > Self.keptFiles, index < order.count {
      let url = order[index]
      guard wanted[url] == nil else {
        index += 1
        continue
      }
      order.remove(at: index)
      if let file = files.removeValue(forKey: url) {
        try? FileManager.default.removeItem(at: file)
      }
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

private actor DownloadSlots {
  private var free: Int
  private var waiters: [CheckedContinuation<Void, Never>] = []

  init(_ count: Int) {
    free = count
  }

  func acquire() async {
    guard free == 0 else {
      free -= 1
      return
    }
    await withCheckedContinuation { waiters.append($0) }
  }

  func release() {
    if waiters.isEmpty {
      free += 1
    } else {
      waiters.removeFirst().resume()
    }
  }
}
