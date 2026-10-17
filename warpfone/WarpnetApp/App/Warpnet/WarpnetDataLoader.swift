import Foundation
import Network
import Nuke

final class WarpnetDataLoader: DataLoading, @unchecked Sendable {
  static let shared = WarpnetDataLoader()

  private let fallback = DataLoader()
  private let lock = NSLock()
  private var client: Client?

  static func install() {
    var configuration = ImagePipeline.Configuration.withDataCache(name: "site.warpnet.warpfone.images")
    configuration.dataLoader = shared
    ImagePipeline.shared = ImagePipeline(configuration: configuration)
  }

  func setClient(_ client: Client?) {
    lock.lock()
    self.client = client
    lock.unlock()
  }

  func loadData(with request: URLRequest,
                didReceiveData: @escaping (Data, URLResponse) -> Void,
                completion: @escaping (Error?) -> Void) -> any Cancellable
  {
    guard let url = request.url, url.scheme == "warpnet" else {
      return fallback.loadData(with: request, didReceiveData: didReceiveData, completion: completion)
    }
    lock.lock()
    let client = client
    lock.unlock()
    let task = Task {
      do {
        guard let client else { throw URLError(.notConnectedToInternet) }
        let data = try await client.mediaData(for: url)
        try Task.checkCancellation()
        didReceiveData(data, URLResponse(url: url, mimeType: nil, expectedContentLength: data.count, textEncodingName: nil))
        completion(nil)
      } catch {
        completion(error)
      }
    }
    return TaskCancellable(task: task)
  }
}

private final class TaskCancellable: Cancellable, @unchecked Sendable {
  private let task: Task<Void, Never>

  init(task: Task<Void, Never>) {
    self.task = task
  }

  func cancel() {
    task.cancel()
  }
}
