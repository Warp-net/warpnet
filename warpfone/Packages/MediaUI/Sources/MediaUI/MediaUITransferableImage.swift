// Modified by Warpnet. See NOTICE.

import CoreTransferable
import Nuke
import SwiftUI
import UIKit

struct MediaUIImageTransferable: Codable, Transferable {
  let url: URL

  func fetchData() async -> Data {
    do {
      return try await ImagePipeline.shared.data(for: url).0
    } catch {
      return Data()
    }
  }

  static var transferRepresentation: some TransferRepresentation {
    DataRepresentation(exportedContentType: .jpeg) { transferable in
      await transferable.fetchData()
    }
  }
}
