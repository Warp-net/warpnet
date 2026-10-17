import SwiftUI
import WarpnetTransport

@MainActor
struct ConnectionBanner: View {
  let monitor: ConnectionMonitor?

  var body: some View {
    if let message {
      HStack(spacing: 6) {
        ProgressView()
          .controlSize(.mini)
        Text(message)
          .font(.footnote)
      }
      .padding(.vertical, 4)
      .frame(maxWidth: .infinity)
      .background(.thinMaterial)
      .transition(.move(edge: .top).combined(with: .opacity))
    }
  }

  private var message: LocalizedStringKey? {
    switch monitor?.linkState {
    case .notConnected:
      "warpnet.connection.offline"
    case let .reconnecting(attempt):
      "warpnet.connection.reconnecting-\(attempt)"
    default:
      nil
    }
  }
}
