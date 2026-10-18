// Modified by Warpnet. See NOTICE.

import Account
import DesignSystem
import Explore
import Foundation
import StatusKit
import SwiftUI

@MainActor
enum Tab: Int, Identifiable, Hashable, CaseIterable, Codable {
  case timeline, notifications, mentions, explore, messages, settings, other
  case profile = 10
  case bookmarks
  case favorites
  case post

  nonisolated var id: Int {
    rawValue
  }

  static var customizable: [Tab] {
    allCases.filter { $0 != .other }
  }

  @ViewBuilder
  func makeContentView(selectedTab: Binding<Tab>, popToRootTab: Binding<Tab>) -> some View {
    switch self {
    case .timeline:
      TimelineTab(popToRootTab: popToRootTab)
    case .notifications:
      NotificationsTab(selectedTab: selectedTab, popToRootTab: popToRootTab, lockedType: nil)
    case .mentions:
      NotificationsTab(selectedTab: selectedTab, popToRootTab: popToRootTab, lockedType: .mention)
    case .explore:
      ExploreTab(popToRootTab: popToRootTab)
    case .messages:
      MessagesTab(popToRootTab: popToRootTab)
    case .settings:
      SettingsTabs(popToRootTab: popToRootTab, isModal: false)
    case .profile:
      ProfileTab(popToRootTab: popToRootTab)
    case .bookmarks:
      NavigationTab {
        AccountStatusesListView(mode: .bookmarks)
      }
    case .favorites:
      NavigationTab {
        AccountStatusesListView(mode: .favorites)
      }
    case .post:
      VStack { }
    case .other:
      EmptyView()
    }
  }

  @ViewBuilder
  var label: some View {
    switch self {
    case .timeline:
      Label("tab.timeline", systemImage: iconName)
    case .notifications:
      Label("tab.notifications", systemImage: iconName)
    case .mentions:
      Label("tab.mentions", systemImage: iconName)
    case .explore:
      Label("tab.explore", systemImage: iconName)
    case .messages:
      Label("tab.messages", systemImage: iconName)
    case .settings:
      Label("tab.settings", systemImage: iconName)
    case .profile:
      Label("tab.profile", systemImage: iconName)
    case .bookmarks:
      Label("accessibility.tabs.profile.picker.bookmarks", systemImage: iconName)
    case .favorites:
      Label("accessibility.tabs.profile.picker.favorites", systemImage: iconName)
    case .post:
      Label("menu.new-post", systemImage: iconName)
    case .other:
      EmptyView()
      
    }
  }

  var iconName: String {
    switch self {
    case .timeline:
      "house"
    case .notifications:
      "bell"
    case .mentions:
      "at"
    case .explore:
      "magnifyingglass"
    case .messages:
      "envelope"
    case .settings:
      "gear"
    case .profile:
      "person"
    case .bookmarks:
      "bookmark"
    case .favorites:
      "heart"
    case .post:
      "square.and.pencil"
    case .other:
      ""
    }
  }
}

@Observable
class SidebarTabs {
  struct SidedebarTab: Hashable, Codable {
    let tab: Tab
    var enabled: Bool
  }
  
  class Storage {
    @AppStorage("sidebar_tabs") var tabs: [SidedebarTab] = [
      .init(tab: .timeline, enabled: true),
      .init(tab: .notifications, enabled: true),
      .init(tab: .mentions, enabled: true),
      .init(tab: .messages, enabled: true),
      .init(tab: .explore, enabled: true),
      .init(tab: .bookmarks, enabled: true),
      .init(tab: .favorites, enabled: true),
      
      .init(tab: .settings, enabled: true),
      .init(tab: .profile, enabled: true),
    ]
  }
  
  private let storage = Storage()
  public static let shared = SidebarTabs()
  
  var tabs: [SidedebarTab] {
    didSet {
      storage.tabs = tabs
    }
  }
  
  func isEnabled(_ tab: Tab) -> Bool {
    tabs.first(where: { $0.tab.id == tab.id })?.enabled == true
  }
  
  private init() {
    tabs = storage.tabs
  }
}

@Observable
class iOSTabs {
  enum TabEntries: String {
    case first, second, third, fourth, fifth
  }
  
  class Storage {
    @AppStorage(TabEntries.first.rawValue) var firstTab = Tab.timeline
    @AppStorage(TabEntries.second.rawValue) var secondTab = Tab.notifications
    @AppStorage(TabEntries.third.rawValue) var thirdTab = Tab.explore
    @AppStorage(TabEntries.fourth.rawValue) var fourthTab = Tab.messages
    @AppStorage(TabEntries.fifth.rawValue) var fifthTab = Tab.profile
  }
  
  private let storage = Storage()
  public static let shared = iOSTabs()
  
  var tabs: [Tab] {
    [firstTab, secondTab, thirdTab, fourthTab, fifthTab]
  }
  
  var firstTab: Tab {
    didSet {
      storage.firstTab = firstTab
    }
  }
  
  var secondTab: Tab {
    didSet {
      storage.secondTab = secondTab
    }
  }
  
  var thirdTab: Tab {
    didSet {
      storage.thirdTab = thirdTab
    }
  }
  
  var fourthTab: Tab {
    didSet {
      storage.fourthTab = fourthTab
    }
  }
  
  var fifthTab: Tab {
    didSet {
      storage.fifthTab = fifthTab
    }
  }
  
  private init() {
    firstTab = storage.firstTab
    secondTab = storage.secondTab
    thirdTab = storage.thirdTab
    fourthTab = storage.fourthTab
    fifthTab = storage.fifthTab
  }
}
