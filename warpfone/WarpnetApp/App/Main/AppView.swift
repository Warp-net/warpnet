// Modified by Warpnet. See NOTICE.

import Account
import AppAccount
import AVFoundation
import DesignSystem
import Env
import MediaUI
import Network
import StatusKit
import SwiftUI
import Timeline

@MainActor
struct AppView: View {
  @Environment(AppAccountsManager.self) private var appAccountsManager
  @Environment(UserPreferences.self) private var userPreferences
  @Environment(Theme.self) private var theme
  @Environment(StreamWatcher.self) private var watcher
  @Environment(PairingModel.self) private var pairing
  
  @Environment(\.openWindow) var openWindow
  @Environment(\.horizontalSizeClass) private var horizontalSizeClass
  @Environment(\.verticalSizeClass) private var verticalSizeClass
  
  @Binding var selectedTab: Tab
  @Binding var appRouterPath: RouterPath
  
  @State var popToRootTab: Tab = .other
  @State var iosTabs = iOSTabs.shared
  @State var sidebarTabs = SidebarTabs.shared
  
  var body: some View {
    Group {
      if appAccountsManager.currentClient.isAuth {
        mainView
      } else {
        PairingView()
      }
    }
    .task {
      await pairing.restoreIfNeeded()
    }
  }

  private var mainView: some View {
    platformView
      .safeAreaInset(edge: .top, spacing: 0) {
        ConnectionBanner(monitor: pairing.monitor)
          .animation(.default, value: pairing.monitor?.linkState)
      }
  }

  private var platformView: some View {
    sidebarView
  }
  
  var availableTabs: [Tab] {
    if !isWideLayout {
      return iosTabs.tabs
    }
    return sidebarTabs.tabs.map{ $0.tab }
  }

  var tabBarView: some View {
    TabView(selection: .init(get: {
      selectedTab
    }, set: { newTab in
      if newTab == .post {
        appRouterPath.presentedSheet = .newStatusEditor
        return
      }
      if newTab == selectedTab {
        /// Stupid hack to trigger onChange binding in tab views.
        popToRootTab = .other
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.01) {
          popToRootTab = selectedTab
        }
      }

      HapticManager.shared.fireHaptic(.tabSelection)

      selectedTab = newTab
    })) {
      ForEach(availableTabs) { tab in
        tab.makeContentView(selectedTab: $selectedTab, popToRootTab: $popToRootTab)
          .toolbar(isWideLayout ? .hidden : .automatic, for: .tabBar)
          .tabItem {
            if userPreferences.showiPhoneTabLabel {
              tab.label
                .environment(\.symbolVariants, tab == selectedTab ? .fill : .none)
            } else {
              Image(systemName: tab.iconName)
            }
          }
          .tag(tab)
          .badge(badgeFor(tab: tab))
          .toolbarBackground(theme.primaryBackgroundColor.opacity(0.30), for: .tabBar)
      }
    }
    .introspect(.tabView, on: .iOS(.v17)) { (tabview: UITabBarController) in
      tabview.tabBar.isHidden = isWideLayout
      tabview.customizableViewControllers = []
      tabview.moreNavigationController.isNavigationBarHidden = true
    }
    .id(appAccountsManager.currentClient.id)
  }

  private func badgeFor(tab: Tab) -> Int {
    if tab == .notifications, selectedTab != tab {
      return watcher.unreadNotificationsCount
    }
    return 0
  }
  
  var sidebarView: some View {
    SideBarView(selectedTab: $selectedTab,
                popToRootTab: $popToRootTab,
                tabs: availableTabs)
    {
      VerticalBarEdgeReader { barEdge in
        HStack(spacing: 0) {
          if showsSecondaryColumn, barEdge == .trailing {
            notificationsSecondaryColumn
            Divider().edgesIgnoringSafeArea(.all)
          }
          tabBarView
          if showsSecondaryColumn, barEdge != .trailing {
            Divider().edgesIgnoringSafeArea(.all)
            notificationsSecondaryColumn
          }
        }
      }
    }
    .environment(appRouterPath)
  }

  // iPhone Plus/Max in landscape is regular width but compact height; it keeps the phone layout.
  private var isWideLayout: Bool {
    horizontalSizeClass == .regular && verticalSizeClass == .regular
  }

  private var showsSecondaryColumn: Bool {
    isWideLayout
      && appAccountsManager.currentClient.isAuth
      && userPreferences.showiPadSecondaryColumn
  }

  var notificationsSecondaryColumn: some View {
    NotificationsTab(selectedTab: .constant(.notifications),
                     popToRootTab: $popToRootTab, lockedType: nil)
      .environment(\.isSecondaryColumn, true)
      .frame(maxWidth: .secondaryColumnWidth)
      .id(appAccountsManager.currentAccount.id)
  }
}
