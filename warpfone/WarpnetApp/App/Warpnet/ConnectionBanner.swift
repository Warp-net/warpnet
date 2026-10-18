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

/// The toolbar dot warpdroid shows next to the title: green while the link to
/// the paired node is up, direct or relayed, red otherwise.
@MainActor
struct ConnectionDot: View {
  let monitor: ConnectionMonitor?

  private var isUp: Bool {
    monitor?.linkState.isUp == true
  }

  var body: some View {
    Circle()
      .fill(isUp ? Color(red: 0x19 / 255, green: 0xA3 / 255, blue: 0x41 / 255)
        : Color(red: 0xDF / 255, green: 0x15 / 255, blue: 0x53 / 255))
      .frame(width: 10, height: 10)
      .accessibilityLabel(isUp ? Text("warpnet.connection.dot.up") : Text("warpnet.connection.dot.down"))
  }
}
