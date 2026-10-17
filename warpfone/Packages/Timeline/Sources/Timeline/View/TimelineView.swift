// Modified by Warpnet. See NOTICE.

import Charts
import DesignSystem
import Env
import Models
import Network
import StatusKit
import SwiftUI
import SwiftUIIntrospect

@MainActor
public struct TimelineView: View {
  @Environment(\.scenePhase) private var scenePhase
  @Environment(Theme.self) private var theme
  @Environment(StreamWatcher.self) private var watcher
  @Environment(Client.self) private var client
  @Environment(RouterPath.self) private var routerPath

  @State private var viewModel = TimelineViewModel()
  @State private var prefetcher = TimelineMediaPrefetcher()
  @State private var contentFilter = TimelineContentFilter.shared

  @State private var wasBackgrounded: Bool = false
  @State private var collectionView: UICollectionView?

  @Binding var timeline: TimelineFilter
  @Binding var scrollToTopSignal: Int

  private let canFilterTimeline: Bool

  public init(timeline: Binding<TimelineFilter>,
              scrollToTopSignal: Binding<Int>,
              canFilterTimeline: Bool)
  {
    _timeline = timeline
    _scrollToTopSignal = scrollToTopSignal
    self.canFilterTimeline = canFilterTimeline
  }

  public var body: some View {
    ScrollViewReader { proxy in
      ZStack(alignment: .top) {
        List {
          scrollToTopView
          StatusesListView(fetcher: viewModel, client: client, routerPath: routerPath)
            .environment(\.isHomeTimeline, timeline == .home)
        }
        .id(client.id)
        .environment(\.defaultMinListRowHeight, 1)
        .listStyle(.plain)
        #if !os(visionOS)
        .scrollContentBackground(.hidden)
        .background(theme.primaryBackgroundColor)
        #endif
        .introspect(.list, on: .iOS(.v17)) { (collectionView: UICollectionView) in
          DispatchQueue.main.async {
            self.collectionView = collectionView
          }
          prefetcher.viewModel = viewModel
          collectionView.isPrefetchingEnabled = true
          collectionView.prefetchDataSource = prefetcher
        }
        if viewModel.timeline.supportNewestPagination {
          TimelineUnreadStatusesView(observer: viewModel.pendingStatusesObserver)
        }
      }
      .onChange(of: viewModel.scrollToIndex) { _, newValue in
        if let collectionView,
           let newValue,
           let rows = collectionView.dataSource?.collectionView(collectionView, numberOfItemsInSection: 0),
           rows > newValue
        {
          collectionView.scrollToItem(at: .init(row: newValue, section: 0),
                                      at: .top,
                                      animated: viewModel.scrollToIndexAnimated)
          viewModel.scrollToIndexAnimated = false
          viewModel.scrollToIndex = nil
        }
      }
      .onChange(of: scrollToTopSignal) {
        withAnimation {
          proxy.scrollTo(ScrollToView.Constants.scrollToTop, anchor: .top)
        }
      }
    }
    .toolbar {
      toolbarTitleView
    }
    .navigationBarTitleDisplayMode(.inline)
    .onAppear {
      viewModel.canFilterTimeline = canFilterTimeline
      viewModel.isTimelineVisible = true

      if viewModel.client == nil {
        viewModel.client = client
      }

      viewModel.timeline = timeline
    }
    .onDisappear {
      viewModel.isTimelineVisible = false
      viewModel.saveMarker()
    }
    .refreshable {
      HapticManager.shared.fireHaptic(.dataRefresh(intensity: 0.3))
      await viewModel.pullToRefresh()
      HapticManager.shared.fireHaptic(.dataRefresh(intensity: 0.7))
    }
    .onChange(of: watcher.latestEvent?.id) {
      Task {
        if let latestEvent = watcher.latestEvent {
          await viewModel.handleEvent(event: latestEvent)
        }
      }
    }
    .onChange(of: timeline) { _, newValue in
      viewModel.client = client
      viewModel.timeline = newValue
    }
    .onChange(of: viewModel.timeline) { _, newValue in
      timeline = newValue
    }
    .onChange(of: contentFilter.showReplies) { _, _ in
      Task { await viewModel.refreshTimelineContentFilter() }
    }
    .onChange(of: contentFilter.showBoosts) { _, _ in
      Task { await viewModel.refreshTimelineContentFilter() }
    }
    .onChange(of: contentFilter.showThreads) { _, _ in
      Task { await viewModel.refreshTimelineContentFilter() }
    }
    .onChange(of: contentFilter.showQuotePosts) { _, _ in
      Task { await viewModel.refreshTimelineContentFilter() }
    }
    .onChange(of: scenePhase) { _, newValue in
      switch newValue {
      case .active:
        if wasBackgrounded {
          wasBackgrounded = false
          viewModel.refreshTimeline()
        }
      case .background:
        wasBackgrounded = true
        viewModel.saveMarker()
        
      default:
        break
      }
    }
  }

  @ToolbarContentBuilder
  private var toolbarTitleView: some ToolbarContent {
    ToolbarItem(placement: .principal) {
      VStack(alignment: .center) {
        Text(timeline.localizedTitle())
          .font(.headline)
      }
      .accessibilityRepresentation {
        if canFilterTimeline {
          Menu(timeline.localizedTitle()) {}
        } else {
          Text(timeline.localizedTitle())
        }
      }
      .accessibilityAddTraits(.isHeader)
      .accessibilityRemoveTraits(.isButton)
      .accessibilityRespondsToUserInteraction(canFilterTimeline)
    }
  }

  private var scrollToTopView: some View {
    ScrollToView()
      .frame(height: .layoutPadding)
      .onAppear {
        viewModel.scrollToTopVisible = true
      }
      .onDisappear {
        viewModel.scrollToTopVisible = false
      }
  }
}
