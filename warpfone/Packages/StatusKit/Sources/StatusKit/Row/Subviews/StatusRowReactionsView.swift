import DesignSystem
import Env
import SwiftUI

/// One chip per emoji somebody put on the tweet, most popular first, the way
/// warpdroid's ReactionChips show them. Tapping a chip reacts with it, and
/// tapping the one you hold takes your reaction back.
@MainActor
struct StatusRowReactionsView: View {
  @Environment(Theme.self) private var theme
  @Environment(StatusDataController.self) private var statusDataController

  var body: some View {
    let chips = statusDataController.reactions
      .filter { $0.value > 0 }
      .sorted { $0.value == $1.value ? $0.key < $1.key : $0.value > $1.value }
    if !chips.isEmpty {
      ReactionsFlowLayout(spacing: 4) {
        ForEach(chips, id: \.key) { emoji, count in
          chip(emoji: emoji, count: count)
        }
      }
    }
  }

  private func chip(emoji: String, count: Int) -> some View {
    let isMine = statusDataController.isFavorited && statusDataController.myReaction == emoji
    return Button {
      Task {
        HapticManager.shared.fireHaptic(.notification(.success))
        await statusDataController.react(emoji: emoji)
      }
    } label: {
      HStack(spacing: 4) {
        Text(emoji)
          .font(.scaledFootnote)
        Text(count, format: .number.notation(.compactName))
          .font(.scaledFootnote)
          .monospacedDigit()
          .foregroundStyle(isMine ? theme.tintColor : .secondary)
      }
      .padding(.horizontal, 8)
      .padding(.vertical, 2)
      .background(Capsule().fill(isMine ? theme.tintColor.opacity(0.15) : theme.secondaryBackgroundColor))
      .overlay(Capsule().strokeBorder(isMine ? theme.tintColor : .secondary.opacity(0.3), lineWidth: 1))
    }
    .buttonStyle(.plain)
    .accessibilityLabel(Text("warpnet.status.reactions-\(count)-\(emoji)"))
  }
}

/// Lays chips out left to right and wraps them onto the next line.
private struct ReactionsFlowLayout: Layout {
  var spacing: CGFloat

  func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache _: inout ()) -> CGSize {
    let rows = arrange(subviews: subviews, width: proposal.width ?? .infinity)
    let height = rows.reduce(0) { $0 + $1.height } + spacing * CGFloat(max(rows.count - 1, 0))
    let width = rows.map(\.width).max() ?? 0
    return CGSize(width: proposal.width ?? width, height: height)
  }

  func placeSubviews(in bounds: CGRect, proposal _: ProposedViewSize, subviews: Subviews, cache _: inout ()) {
    var y = bounds.minY
    for row in arrange(subviews: subviews, width: bounds.width) {
      var x = bounds.minX
      for index in row.indices {
        let size = subviews[index].sizeThatFits(.unspecified)
        subviews[index].place(at: CGPoint(x: x, y: y), proposal: ProposedViewSize(size))
        x += size.width + spacing
      }
      y += row.height + spacing
    }
  }

  private struct Row {
    var indices: [Int] = []
    var width: CGFloat = 0
    var height: CGFloat = 0
  }

  private func arrange(subviews: Subviews, width: CGFloat) -> [Row] {
    var rows: [Row] = []
    var current = Row()
    for index in subviews.indices {
      let size = subviews[index].sizeThatFits(.unspecified)
      let needed = current.indices.isEmpty ? size.width : current.width + spacing + size.width
      if needed > width, !current.indices.isEmpty {
        rows.append(current)
        current = Row()
      }
      current.width = current.indices.isEmpty ? size.width : current.width + spacing + size.width
      current.height = max(current.height, size.height)
      current.indices.append(index)
    }
    if !current.indices.isEmpty {
      rows.append(current)
    }
    return rows
  }
}
