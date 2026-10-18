import DesignSystem
import Env
import Models
import SwiftUI

@MainActor
struct StatusRowSwipeView: View {
  @Environment(Theme.self) private var theme
  @Environment(UserPreferences.self) private var preferences
  @Environment(StatusDataController.self) private var statusDataController

  enum Mode {
    case leading, trailing
  }

  var viewModel: StatusRowViewModel
  let mode: Mode

  var body: some View {
    switch mode {
    case .leading:
      leadingSwipeActions
    case .trailing:
      trailingSwipeActions
    }
  }

  @ViewBuilder
  private var trailingSwipeActions: some View {
    if preferences.swipeActionsStatusTrailingRight != StatusAction.none {
      makeSwipeButton(action: preferences.swipeActionsStatusTrailingRight)
        .tint(preferences.swipeActionsStatusTrailingRight.color(themeTintColor: theme.tintColor, useThemeColor: preferences.swipeActionsUseThemeColor, outside: true))
    }
    if preferences.swipeActionsStatusTrailingLeft != StatusAction.none {
      makeSwipeButton(action: preferences.swipeActionsStatusTrailingLeft)
        .tint(preferences.swipeActionsStatusTrailingLeft.color(themeTintColor: theme.tintColor, useThemeColor: preferences.swipeActionsUseThemeColor, outside: false))
    }
  }

  @ViewBuilder
  private var leadingSwipeActions: some View {
    if preferences.swipeActionsStatusLeadingLeft != StatusAction.none {
      makeSwipeButton(action: preferences.swipeActionsStatusLeadingLeft)
        .tint(preferences.swipeActionsStatusLeadingLeft.color(themeTintColor: theme.tintColor, useThemeColor: preferences.swipeActionsUseThemeColor, outside: true))
    }
    if preferences.swipeActionsStatusLeadingRight != StatusAction.none {
      makeSwipeButton(action: preferences.swipeActionsStatusLeadingRight)
        .tint(preferences.swipeActionsStatusLeadingRight.color(themeTintColor: theme.tintColor, useThemeColor: preferences.swipeActionsUseThemeColor, outside: false))
    }
  }

  @ViewBuilder
  private func makeSwipeButton(action: StatusAction) -> some View {
    switch action {
    case .reply:
      makeSwipeButtonForRouterPath(action: action, destination: .replyToStatusEditor(status: viewModel.status))
    case .quote:
      makeSwipeButtonForRouterPath(action: action, destination: .quoteStatusEditor(status: viewModel.status))
    case .favorite:
      makeSwipeButtonForTask(action: action) {
        await statusDataController.toggleFavorite()
      }
    case .boost:
      makeSwipeButtonForTask(action: action) {
        await statusDataController.toggleReblog()
      }
    case .bookmark:
      makeSwipeButtonForTask(action: action) {
        await statusDataController.toggleBookmark()
      }
    case .none:
      EmptyView()
    }
  }

  @ViewBuilder
  private func makeSwipeButtonForRouterPath(action: StatusAction, destination: SheetDestination) -> some View {
    Button {
      HapticManager.shared.fireHaptic(.notification(.success))
      viewModel.routerPath.presentedSheet = destination
    } label: {
      makeSwipeLabel(action: action, style: preferences.swipeActionsIconStyle)
    }
  }

  @ViewBuilder
  private func makeSwipeButtonForTask(action: StatusAction, task: @escaping () async -> Void) -> some View {
    Button {
      Task {
        HapticManager.shared.fireHaptic(.notification(.success))
        await task()
      }
    } label: {
      makeSwipeLabel(action: action, style: preferences.swipeActionsIconStyle)
    }
  }

  @ViewBuilder
  private func makeSwipeLabel(action: StatusAction, style: UserPreferences.SwipeActionsIconStyle) -> some View {
    switch style {
    case .iconOnly:
      Label(action.displayName(isReblogged: statusDataController.isReblogged,
                               isFavorited: statusDataController.isFavorited,
                               isBookmarked: statusDataController.isBookmarked),
            imageNamed: action.iconName(isReblogged: statusDataController.isReblogged,
                                        isFavorited: statusDataController.isFavorited,
                                        isBookmarked: statusDataController.isBookmarked))
        .labelStyle(.iconOnly)
        .environment(\.symbolVariants, .none)
    case .iconWithText:
      Label(action.displayName(isReblogged: statusDataController.isReblogged,
                               isFavorited: statusDataController.isFavorited,
                               isBookmarked: statusDataController.isBookmarked),
            imageNamed: action.iconName(isReblogged: statusDataController.isReblogged,
                                        isFavorited: statusDataController.isFavorited,
                                        isBookmarked: statusDataController.isBookmarked))
        .labelStyle(.titleAndIcon)
        .environment(\.symbolVariants, .none)
    }
  }
}
