import DesignSystem
import Env
import Models
import Network
import SwiftUI

@MainActor
public struct StatusesListView<Fetcher>: View where Fetcher: StatusesFetcher {
  @Environment(Theme.self) private var theme

  @State private var fetcher: Fetcher
  private let routerPath: RouterPath
  private let client: Client
  private let interstitial: AnyView?

  /// Rows the interstitial follows, or the end of a shorter list.
  private let interstitialPosition = 5

  public init(fetcher: Fetcher,
              client: Client,
              routerPath: RouterPath,
              interstitial: AnyView? = nil)
  {
    _fetcher = .init(initialValue: fetcher)
    self.client = client
    self.routerPath = routerPath
    self.interstitial = interstitial
  }

  public var body: some View {
    switch fetcher.statusesState {
    case .loading:
      ForEach(Status.placeholders()) { status in
        StatusRowView(viewModel: .init(status: status, client: client, routerPath: routerPath))
          .redacted(reason: .placeholder)
          .allowsHitTesting(false)
      }
    case .error:
      ErrorView(title: "status.error.title",
                message: "status.error.loading.message",
                buttonTitle: "action.retry")
      {
        Task {
          await fetcher.fetchNewestStatuses(pullToRefresh: false)
        }
      }
      .listRowBackground(theme.primaryBackgroundColor)
      .listRowSeparator(.hidden)

    case let .display(statuses, nextPageState):
      ForEach(Array(statuses.enumerated()), id: \.element.id) { index, status in
        StatusRowView(viewModel: StatusRowViewModel(status: status,
                                                    client: client,
                                                    routerPath: routerPath))
          .onAppear {
            fetcher.statusDidAppear(status: status)
          }
          .onDisappear {
            fetcher.statusDidDisappear(status: status)
          }
        if index == min(interstitialPosition, statuses.count) - 1, let interstitial {
          interstitialRow(interstitial)
        }
      }
      if statuses.isEmpty, let interstitial {
        interstitialRow(interstitial)
      }
      switch nextPageState {
      case .hasNextPage:
        loadingRow
          .id(UUID().uuidString)
          .onAppear {
            Task {
              await fetcher.fetchNextPage()
            }
          }
      case .loadingNextPage:
        loadingRow
          .id(UUID().uuidString)
      case .none:
        EmptyView()
      }
    }
  }

  private func interstitialRow(_ content: AnyView) -> some View {
    content
      .listRowInsets(EdgeInsets())
      .listRowSeparator(.hidden)
      .listRowBackground(theme.primaryBackgroundColor)
  }

  private var loadingRow: some View {
    HStack {
      Spacer()
      ProgressView()
      Spacer()
    }
    .padding(.horizontal, .layoutPadding)
    .listRowBackground(theme.primaryBackgroundColor)
  }
}
