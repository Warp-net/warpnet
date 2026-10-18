// Modified by Warpnet. See NOTICE.

import DesignSystem
import Env
import Models
import Network
import SwiftUI

@MainActor
struct StatusRowActionsView: View {
  @Environment(Theme.self) private var theme
  @Environment(StatusDataController.self) private var statusDataController

  @Environment(\.openWindow) private var openWindow
  @Environment(\.isStatusFocused) private var isFocused
  @Environment(\.horizontalSizeClass) var horizontalSizeClass

  @State private var showTextForSelection: Bool = false

  @Binding var isBlockConfirmationPresented: Bool

  var viewModel: StatusRowViewModel

  var isNarrow: Bool {
    horizontalSizeClass == .compact && (UIDevice.current.userInterfaceIdiom == .pad || UIDevice.current.userInterfaceIdiom == .mac)
  }

  let actions: [Action] = [.respond, .boost, .favorite, .bookmark, .menu]

  @MainActor
  enum Action {
    case respond, boost, favorite, bookmark, menu

    func image(dataController: StatusDataController) -> Image {
      switch self {
      case .respond:
        Image(systemName: "arrowshape.turn.up.left")
      case .boost:
        Image(systemName: "arrow.2.squarepath")
      case .favorite:
        Image(systemName: dataController.isFavorited ? "heart.fill" : "heart")
      case .bookmark:
        Image(systemName: dataController.isBookmarked ? "bookmark.fill" : "bookmark")
      case .menu:
        Image(systemName: "ellipsis")
      }
    }

    func accessibilityLabel(dataController: StatusDataController) -> LocalizedStringKey {
      switch self {
      case .respond:
        "status.action.reply"
      case .boost:
        dataController.isReblogged ? "status.action.unboost" : "status.action.boost"
      case .favorite:
        dataController.isFavorited ? "status.action.unfavorite" : "status.action.favorite"
      case .bookmark:
        dataController.isBookmarked ? "status.action.unbookmark" : "status.action.bookmark"
      case .menu:
        "status.action.context-menu"
      }
    }

    func count(dataController: StatusDataController, isFocused: Bool, theme: Theme) -> Int? {
      if theme.statusActionsDisplay == .discret, !isFocused {
        return nil
      }
      switch self {
      case .respond:
        return dataController.repliesCount
      case .favorite:
        return dataController.favoritesCount
      case .boost:
        return dataController.reblogsCount
      case .bookmark, .menu:
        return nil
      }
    }

    func tintColor(theme: Theme) -> Color? {
      switch self {
      case .respond, .menu:
        nil
      case .favorite:
        .red
      case .bookmark:
        theme.bookmarkColor
      case .boost:
        theme.tintColor
      }
    }

    func isOn(dataController: StatusDataController) -> Bool {
      switch self {
      case .respond, .menu: false
      case .favorite: dataController.isFavorited
      case .bookmark: dataController.isBookmarked
      case .boost: dataController.isReblogged
      }
    }
  }

  var body: some View {
    VStack(spacing: 12) {
      HStack {
        ForEach(actions, id: \.self) { action in
          if action == .menu {
            Menu {
              StatusRowContextMenu(viewModel: viewModel,
                                   showTextForSelection: $showTextForSelection,
                                   isBlockConfirmationPresented: $isBlockConfirmationPresented)
                .onAppear {
                  Task {
                    await viewModel.loadAuthorRelationship()
                  }
                }
            } label: {
              Label("", systemImage: "ellipsis")
                .padding(.vertical, 6)
            }
            .menuStyle(.button)
            .buttonStyle(.borderless)
            .foregroundStyle(.secondary)
            .contentShape(Rectangle())
            .accessibilityLabel("status.action.context-menu")
          } else {
            actionButton(action: action)
            Spacer()
          }
        }
      }
    }
    .sheet(isPresented: $showTextForSelection) {
      let content = viewModel.status.reblog?.content.asSafeMarkdownAttributedString ?? viewModel.status.content.asSafeMarkdownAttributedString
      SelectTextView(content: content)
    }
  }

  @ViewBuilder
  private func actionButton(action: Action) -> some View {
    if action == .boost {
      styled(Menu {
        Button {
          handleAction(action: action)
        } label: {
          Label(statusDataController.isReblogged ? "status.action.unboost" : "status.action.boost",
                systemImage: "arrow.2.squarepath")
        }
        Button {
          #if targetEnvironment(macCatalyst)
            openWindow(value: WindowDestinationEditor.quoteStatusEditor(status: viewModel.status))
          #else
            viewModel.routerPath.presentedSheet = .quoteStatusEditor(status: viewModel.status)
          #endif
        } label: {
          Label("status.action.quote", systemImage: "quote.bubble")
        }
      } label: {
        actionLabel(action: action)
      }
      .menuStyle(.button), action: action)
    } else if action == .favorite {
      // A context menu on the button loses the long press to the row's own.
      styled(Menu {
        ForEach(WarpnetReactions.quick, id: \.self) { emoji in
          reactionButton(emoji)
        }
        Menu {
          ForEach(WarpnetReactions.more, id: \.self) { emoji in
            reactionButton(emoji)
          }
        } label: {
          Label("warpnet.status.action.more-reactions", systemImage: "plus")
        }
      } label: {
        actionLabel(action: action)
      } primaryAction: {
        handleAction(action: action)
      }
      .menuStyle(.button), action: action)
    } else {
      styled(Button {
        handleAction(action: action)
      } label: {
        actionLabel(action: action)
      }, action: action)
    }
  }

  private func reactionButton(_ emoji: String) -> some View {
    Button(emoji) {
      Task {
        HapticManager.shared.fireHaptic(.notification(.success))
        await statusDataController.react(emoji: emoji)
      }
    }
  }

  private func actionLabel(action: Action) -> some View {
    HStack(spacing: 2) {
      if action == .favorite, statusDataController.isFavorited, let reaction = statusDataController.myReaction {
        Text(reaction)
          #if targetEnvironment(macCatalyst)
          .font(.scaledBody)
          #else
          .font(.body)
          .dynamicTypeSize(.large)
          #endif
      } else {
        action
          .image(dataController: statusDataController)
          .fontWeight(action == .boost ? .semibold : nil)
          #if targetEnvironment(macCatalyst)
          .font(.scaledBody)
          #else
          .font(.body)
          .dynamicTypeSize(.large)
          #endif
      }
      if !isNarrow,
         let count = action.count(dataController: statusDataController,
                                  isFocused: isFocused,
                                  theme: theme)
      {
        Text(count, format: .number.notation(.compactName))
          .lineLimit(1)
          .minimumScaleFactor(0.6)
          .contentTransition(.numericText(value: Double(count)))
          .foregroundColor(Color(UIColor.secondaryLabel))
          #if targetEnvironment(macCatalyst)
          .font(.scaledFootnote)
          #else
          .font(.footnote)
          .dynamicTypeSize(.medium)
          #endif
          .monospacedDigit()
          .opacity(count > 0 ? 1 : 0)
      }
    }
    .padding(.vertical, 6)
    .padding(.horizontal, 8)
    .contentShape(Rectangle())
  }

  private func styled(_ content: some View, action: Action) -> some View {
    content
      .buttonStyle(
        .statusAction(
          isOn: action.isOn(dataController: statusDataController),
          tintColor: action.tintColor(theme: theme)
        )
      )
      .offset(x: -8)
      .accessibilityElement(children: .combine)
      .accessibilityLabel(action.accessibilityLabel(dataController: statusDataController))
  }

  private func handleAction(action: Action) {
    Task {
      HapticManager.shared.fireHaptic(.notification(.success))
      switch action {
      case .respond:
        #if targetEnvironment(macCatalyst)
          openWindow(value: WindowDestinationEditor.replyToStatusEditor(status: viewModel.status))
        #else
          viewModel.routerPath.presentedSheet = .replyToStatusEditor(status: viewModel.status)
        #endif
      case .favorite:
        await statusDataController.toggleFavorite()
      case .bookmark:
        await statusDataController.toggleBookmark()
      case .boost:
        await statusDataController.toggleReblog()
      case .menu:
        break
      }
    }
  }
}
