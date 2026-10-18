import DesignSystem
import Foundation
import SwiftUI
import WarpnetTransport

/// The last lines the embedded node logged, kept for the Logs screen the way
/// warpdroid's log viewer keeps them. Written from any node thread.
final class NodeLogBuffer: @unchecked Sendable {
  static let shared = NodeLogBuffer()
  static let capacity = 2000

  private let lock = NSLock()
  private var lines: [String] = []
  private let formatter: DateFormatter = {
    let formatter = DateFormatter()
    formatter.dateFormat = "HH:mm:ss.SSS"
    return formatter
  }()

  func append(level: NodeLogLevel, component: String, message: String) {
    lock.lock()
    defer { lock.unlock() }
    lines.append("\(formatter.string(from: Date())) \(level.letter) \(component): \(message)")
    if lines.count > Self.capacity {
      lines.removeFirst(lines.count - Self.capacity)
    }
  }

  func snapshot() -> [String] {
    lock.lock()
    defer { lock.unlock() }
    return lines
  }

  func clear() {
    lock.lock()
    defer { lock.unlock() }
    lines.removeAll()
  }
}

private extension NodeLogLevel {
  var letter: String {
    switch self {
    case .debug: "D"
    case .info: "I"
    case .warn: "W"
    case .error: "E"
    }
  }
}

@MainActor
struct LogsView: View {
  static let verboseKey = "warpnet.logs.verbose"

  @Environment(Theme.self) private var theme
  @Environment(PairingModel.self) private var pairing

  @AppStorage(Self.verboseKey) private var isVerbose = false
  @AppStorage("warpnet.logs.autoscroll") private var autoscroll = true
  @State private var lines: [String] = []

  var body: some View {
    ScrollViewReader { proxy in
      List {
        ForEach(Array(lines.enumerated()), id: \.offset) { index, line in
          Text(line)
            .font(.system(.caption2, design: .monospaced))
            .textSelection(.enabled)
            .listRowBackground(theme.primaryBackgroundColor)
            .id(index)
        }
      }
      .listStyle(.plain)
      .defaultScrollAnchor(autoscroll ? .bottom : .top)
      .overlay {
        if lines.isEmpty {
          Text("warpnet.logs.empty")
            .foregroundStyle(.secondary)
        }
      }
      .onChange(of: lines) { _, lines in
        if autoscroll, !lines.isEmpty {
          proxy.scrollTo(lines.count - 1, anchor: .bottom)
        }
      }
    }
    .scrollContentBackground(.hidden)
    .background(theme.secondaryBackgroundColor)
    .navigationTitle("warpnet.logs.title")
    .navigationBarTitleDisplayMode(.inline)
    .toolbar {
      ToolbarItem(placement: .navigationBarTrailing) {
        Menu {
          Toggle("warpnet.logs.autoscroll", isOn: $autoscroll)
          Toggle("warpnet.logs.verbose", isOn: $isVerbose)
          Button {
            UIPasteboard.general.string = lines.joined(separator: "\n")
          } label: {
            Label("warpnet.logs.copy-all", systemImage: "doc.on.doc")
          }
          Button(role: .destructive) {
            NodeLogBuffer.shared.clear()
            lines = []
          } label: {
            Label("warpnet.logs.clear", systemImage: "trash")
          }
        } label: {
          Image(systemName: "ellipsis.circle")
        }
      }
    }
    .onChange(of: isVerbose) { _, verbose in
      Task {
        await pairing.session?.client.setNodeLogMinLevel(verbose ? .debug : .info)
      }
    }
    .task {
      while !Task.isCancelled {
        let snapshot = NodeLogBuffer.shared.snapshot()
        if snapshot != lines {
          lines = snapshot
        }
        try? await Task.sleep(for: .seconds(1))
      }
    }
  }
}
