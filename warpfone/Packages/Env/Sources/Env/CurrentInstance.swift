import Combine
import Foundation
import Models
import Network
import Observation

@MainActor
@Observable public class CurrentInstance {
  public private(set) var instance: Instance?

  private var client: Client?

  public static let shared = CurrentInstance()

  private init() {}

  public func setClient(client: Client) {
    self.client = client
  }

  public func fetchCurrentInstance() async {
    guard let client else { return }
    instance = try? await client.get(endpoint: Instances.instance)
  }
}
