// Modified by Warpnet. See NOTICE.

import Combine
import Foundation
import Models
import Network
import SwiftUI

@MainActor
@Observable public class UserPreferences {
  class Storage {
    @AppStorage("preferred_browser") public var preferredBrowser: PreferredBrowser = .inAppSafari
    @AppStorage("show_pending_at_bottom") public var pendingShownAtBottom: Bool = false
    @AppStorage("show_pending_left") public var pendingShownLeft: Bool = false

    @AppStorage("social_keyboard_composer") public var isSocialKeyboardEnabled: Bool = false

    @AppStorage("autoplay_video") public var autoPlayVideo = true
    @AppStorage("mute_video") public var muteVideo = true

    @AppStorage("inAppBrowserReaderView") public var inAppBrowserReaderView = false

    @AppStorage("haptic_tab") public var hapticTabSelectionEnabled = true
    @AppStorage("haptic_timeline") public var hapticTimelineEnabled = true
    @AppStorage("haptic_button_press") public var hapticButtonPressEnabled = true

    @AppStorage("show_tab_label_iphone") public var showiPhoneTabLabel = true

    @AppStorage("show_second_column_ipad") public var showiPadSecondaryColumn = true

    @AppStorage("swipeactions-status-trailing-right") public var swipeActionsStatusTrailingRight = StatusAction.favorite
    @AppStorage("swipeactions-status-trailing-left") public var swipeActionsStatusTrailingLeft = StatusAction.boost
    @AppStorage("swipeactions-status-leading-left") public var swipeActionsStatusLeadingLeft = StatusAction.reply
    @AppStorage("swipeactions-status-leading-right") public var swipeActionsStatusLeadingRight = StatusAction.none
    @AppStorage("swipeactions-use-theme-color") public var swipeActionsUseThemeColor = false
    @AppStorage("swipeactions-icon-style") public var swipeActionsIconStyle: SwipeActionsIconStyle = .iconWithText

    @AppStorage("collapse-long-posts") public var collapseLongPosts = true

    @AppStorage("max_reply_indentation") public var maxReplyIndentation: UInt = 7
    @AppStorage("show_reply_indentation") public var showReplyIndentation: Bool = true

    @AppStorage("show_account_popover") public var showAccountPopover: Bool = true

    init() {}
  }

  public static let shared = UserPreferences()
  private let storage = Storage()

  public var preferredBrowser: PreferredBrowser {
    didSet {
      storage.preferredBrowser = preferredBrowser
    }
  }

  public var pendingShownAtBottom: Bool {
    didSet {
      storage.pendingShownAtBottom = pendingShownAtBottom
    }
  }

  public var pendingShownLeft: Bool {
    didSet {
      storage.pendingShownLeft = pendingShownLeft
    }
  }

  public var pendingLocation: Alignment {
    let fromLeft = Locale.current.language.characterDirection == .leftToRight ? pendingShownLeft : !pendingShownLeft
    if pendingShownAtBottom {
      if fromLeft {
        return .bottomLeading
      } else {
        return .bottomTrailing
      }
    } else {
      if fromLeft {
        return .topLeading
      } else {
        return .topTrailing
      }
    }
  }

  public var isSocialKeyboardEnabled: Bool {
    didSet {
      storage.isSocialKeyboardEnabled = isSocialKeyboardEnabled
    }
  }

  public var autoPlayVideo: Bool {
    didSet {
      storage.autoPlayVideo = autoPlayVideo
    }
  }

  public var muteVideo: Bool {
    didSet {
      storage.muteVideo = muteVideo
    }
  }

  public var inAppBrowserReaderView: Bool {
    didSet {
      storage.inAppBrowserReaderView = inAppBrowserReaderView
    }
  }

  public var hapticTabSelectionEnabled: Bool {
    didSet {
      storage.hapticTabSelectionEnabled = hapticTabSelectionEnabled
    }
  }

  public var hapticTimelineEnabled: Bool {
    didSet {
      storage.hapticTimelineEnabled = hapticTimelineEnabled
    }
  }

  public var hapticButtonPressEnabled: Bool {
    didSet {
      storage.hapticButtonPressEnabled = hapticButtonPressEnabled
    }
  }

  public var showiPhoneTabLabel: Bool {
    didSet {
      storage.showiPhoneTabLabel = showiPhoneTabLabel
    }
  }

  public var showiPadSecondaryColumn: Bool {
    didSet {
      storage.showiPadSecondaryColumn = showiPadSecondaryColumn
    }
  }

  public var swipeActionsStatusTrailingRight: StatusAction {
    didSet {
      storage.swipeActionsStatusTrailingRight = swipeActionsStatusTrailingRight
    }
  }

  public var swipeActionsStatusTrailingLeft: StatusAction {
    didSet {
      storage.swipeActionsStatusTrailingLeft = swipeActionsStatusTrailingLeft
    }
  }

  public var swipeActionsStatusLeadingLeft: StatusAction {
    didSet {
      storage.swipeActionsStatusLeadingLeft = swipeActionsStatusLeadingLeft
    }
  }

  public var swipeActionsStatusLeadingRight: StatusAction {
    didSet {
      storage.swipeActionsStatusLeadingRight = swipeActionsStatusLeadingRight
    }
  }

  public var swipeActionsUseThemeColor: Bool {
    didSet {
      storage.swipeActionsUseThemeColor = swipeActionsUseThemeColor
    }
  }

  public var swipeActionsIconStyle: SwipeActionsIconStyle {
    didSet {
      storage.swipeActionsIconStyle = swipeActionsIconStyle
    }
  }

  public var collapseLongPosts: Bool {
    didSet {
      storage.collapseLongPosts = collapseLongPosts
    }
  }

  public var maxReplyIndentation: UInt {
    didSet {
      storage.maxReplyIndentation = maxReplyIndentation
    }
  }

  public var showReplyIndentation: Bool {
    didSet {
      storage.showReplyIndentation = showReplyIndentation
    }
  }

  public var showAccountPopover: Bool {
    didSet {
      storage.showAccountPopover = showAccountPopover
    }
  }

  public func getRealMaxIndent() -> UInt {
    showReplyIndentation ? maxReplyIndentation : 0
  }

  public enum SwipeActionsIconStyle: String, CaseIterable {
    case iconWithText, iconOnly

    public var description: LocalizedStringKey {
      switch self {
      case .iconWithText:
        "enum.swipeactions.icon-with-text"
      case .iconOnly:
        "enum.swipeactions.icon-only"
      }
    }

    // Have to implement this manually here due to compiler not implicitly
    // inserting `nonisolated`, which leads to a warning:
    //
    //     Main actor-isolated static property 'allCases' cannot be used to
    //     satisfy nonisolated protocol requirement
    //
    public nonisolated static var allCases: [Self] {
      [.iconWithText, .iconOnly]
    }
  }

  private init() {
    preferredBrowser = storage.preferredBrowser
    isSocialKeyboardEnabled = storage.isSocialKeyboardEnabled
    autoPlayVideo = storage.autoPlayVideo
    inAppBrowserReaderView = storage.inAppBrowserReaderView
    hapticTabSelectionEnabled = storage.hapticTabSelectionEnabled
    hapticTimelineEnabled = storage.hapticTimelineEnabled
    hapticButtonPressEnabled = storage.hapticButtonPressEnabled
    showiPhoneTabLabel = storage.showiPhoneTabLabel
    showiPadSecondaryColumn = storage.showiPadSecondaryColumn
    swipeActionsStatusTrailingRight = storage.swipeActionsStatusTrailingRight
    swipeActionsStatusTrailingLeft = storage.swipeActionsStatusTrailingLeft
    swipeActionsStatusLeadingLeft = storage.swipeActionsStatusLeadingLeft
    swipeActionsStatusLeadingRight = storage.swipeActionsStatusLeadingRight
    swipeActionsUseThemeColor = storage.swipeActionsUseThemeColor
    swipeActionsIconStyle = storage.swipeActionsIconStyle
    collapseLongPosts = storage.collapseLongPosts
    pendingShownAtBottom = storage.pendingShownAtBottom
    pendingShownLeft = storage.pendingShownLeft
    maxReplyIndentation = storage.maxReplyIndentation
    showReplyIndentation = storage.showReplyIndentation
    showAccountPopover = storage.showAccountPopover
    muteVideo = storage.muteVideo
  }
}

extension UInt: RawRepresentable {
  public var rawValue: Int {
    Int(self)
  }

  public init?(rawValue: Int) {
    if rawValue >= 0 {
      self.init(rawValue)
    } else {
      return nil
    }
  }
}
