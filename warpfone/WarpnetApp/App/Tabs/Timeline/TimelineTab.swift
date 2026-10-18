// Modified by Warpnet. See NOTICE.

import AppAccount
import Combine
import DesignSystem
import Env
import Models
import Network
import SwiftUI
import Timeline

@MainActor
struct TimelineTab: View {
  @Environment(AppAccountsManager.self) private var appAccount
  @Environment(Theme.self) private var theme
  @Environment(CurrentAccount.self) private var currentAccount
  @Environment(UserPreferences.self) private var preferences
  @Environment(Client.self) private var client
  @State private var routerPath = RouterPath()
  @Binding var popToRootTab: Tab

  @State private var didAppear: Bool = false
  @State private var timeline: TimelineFilter = .home
  @State private var scrollToTopSignal: Int = 0

  @AppStorage("last_timeline_filter") var lastTimelineFilter: TimelineFilter = .home

  private let canFilterTimeline: Bool

  init(popToRootTab: Binding<Tab>, timeline: TimelineFilter? = nil) {
    canFilterTimeline = timeline == nil
    _popToRootTab = popToRootTab
    _timeline = .init(initialValue: timeline ?? .home)
  }

  var body: some View {
    NavigationStack(path: $routerPath.path) {
      TimelineView(timeline: $timeline,
                   scrollToTopSignal: $scrollToTopSignal,
                   canFilterTimeline: canFilterTimeline)
        .withAppRouter()
        .withSheetDestinations(sheetDestinations: $routerPath.presentedSheet)
        .toolbar {
          toolbarView
        }
        .toolbarBackground(theme.primaryBackgroundColor.opacity(0.30), for: .navigationBar)
        .id(client.id)
    }
    .onAppear {
      routerPath.client = client
      if !didAppear, canFilterTimeline {
        didAppear = true
        if client.isAuth {
          timeline = lastTimelineFilter
        }
      }
    }
    .onChange(of: client.isAuth) {
      resetTimelineFilter()
    }
    .onChange(of: currentAccount.account?.id) {
      resetTimelineFilter()
    }
    .onChange(of: $popToRootTab.wrappedValue) { _, newValue in
      if newValue == .timeline {
        if routerPath.path.isEmpty {
          scrollToTopSignal += 1
        } else {
          routerPath.path = []
        }
      }
    }
    .onChange(of: client.id) {
      routerPath.path = []
    }
    .onChange(of: timeline) { _, newValue in
      if client.isAuth, canFilterTimeline {
        lastTimelineFilter = newValue
      }
    }
    .onReceive(NotificationCenter.default.publisher(for: .refreshTimeline)) { _ in
      timeline = .latest
    }
    .onReceive(NotificationCenter.default.publisher(for: .homeTimeline)) { _ in
      timeline = .home
    }
    .withSafariRouter()
    .environment(routerPath)
  }

  @ViewBuilder
  private var timelineFilterButton: some View {
    headerGroup
    Divider()
    contentFilterButton
  }

  @ToolbarContentBuilder
  private var toolbarView: some ToolbarContent {
    if canFilterTimeline {
      ToolbarTitleMenu {
        timelineFilterButton
      }
    }
    ToolbarTab(routerPath: $routerPath)
  }

  @ViewBuilder
  private var headerGroup: some View {
    ControlGroup {
      if timeline.supportNewestPagination {
        Button {
          timeline = .latest
        } label: {
          Label(TimelineFilter.latest.localizedTitle(), systemImage: TimelineFilter.latest.iconName())
        }
      }
    }
  }

  private var contentFilterButton: some View {
    Button(action: {
      routerPath.presentedSheet = .timelineContentFilter
    }, label: {
      Label("timeline.content-filter.title", systemImage: "line.3.horizontal.decrease")
    })
  }

  private func resetTimelineFilter() {
    if client.isAuth, canFilterTimeline {
      timeline = lastTimelineFilter
    }
  }
}
