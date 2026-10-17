// Modified by Warpnet. See NOTICE.

import Foundation

public struct LinkHandler {
  public let rawLink: String

  public init(rawLink: String) {
    self.rawLink = rawLink
  }

  public init(maxId: String) {
    let encoded = maxId.addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? maxId
    rawLink = "<warpnet://next?max_id=\(encoded)>; rel=\"next\""
  }

  public var maxId: String? {
    guard let range = rawLink.range(of: #"max_id=[^&>;\s]+"#, options: .regularExpression) else { return nil }
    let value = String(rawLink[range].dropFirst("max_id=".count))
    return value.removingPercentEncoding ?? value
  }
}

extension LinkHandler: Sendable {}
