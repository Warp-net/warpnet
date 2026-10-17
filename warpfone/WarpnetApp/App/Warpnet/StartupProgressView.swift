import DesignSystem
import SwiftUI

struct StartupProgressView: View {
  private let fillDuration: TimeInterval = 10
  private let cap = 0.9

  @State private var startedAt = Date()

  private var stages: [LocalizedStringKey] {
    [
      "warpnet.startup.node",
      "warpnet.startup.peers",
      "warpnet.startup.handshake",
      "warpnet.startup.pairing",
      "warpnet.startup.connected",
      "warpnet.startup.account",
      "warpnet.startup.timeline",
    ]
  }

  var body: some View {
    TimelineView(.periodic(from: .now, by: 0.05)) { context in
      let fraction = min(context.date.timeIntervalSince(startedAt) / fillDuration, 1)
      let stage = min(Int(fraction * Double(stages.count)), stages.count - 1)
      VStack(spacing: 16) {
        ProgressView(value: fraction * cap)
        Text(stages[stage])
          .font(.scaledSubheadline)
          .foregroundStyle(.secondary)
      }
      .padding(32)
      .frame(maxWidth: 480)
    }
    .onAppear {
      startedAt = Date()
    }
  }
}
