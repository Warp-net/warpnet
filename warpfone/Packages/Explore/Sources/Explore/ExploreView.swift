// Modified by Warpnet. See NOTICE.

import Account
import DesignSystem
import Env
import Models
import Network
import StatusKit
import SwiftUI

@MainActor
public struct ExploreView: View {
  @Environment(Theme.self) private var theme
  @Environment(Client.self) private var client
  @Environment(RouterPath.self) private var routerPath

  @State private var viewModel = ExploreViewModel()

  @Binding var scrollToTopSignal: Int

  public init(scrollToTopSignal: Binding<Int>) {
    _scrollToTopSignal = scrollToTopSignal
  }

  public var body: some View {
    ScrollViewReader { proxy in
      List {
        scrollToTopView
          .padding(.bottom, 4)
        if !viewModel.isLoaded {
          loadingView
        } else if !viewModel.searchQuery.isEmpty {
          if let results = viewModel.results[viewModel.searchQuery] {
            if results.isEmpty, !viewModel.isSearching {
              EmptyView(iconName: "magnifyingglass",
                        title: "explore.search.empty.title",
                        message: "explore.search.empty.message")
                .listRowBackground(theme.secondaryBackgroundColor)
                .listRowSeparator(.hidden)
            } else {
              makeSearchResultsView(results: results)
            }
          } else {
            HStack {
              Spacer()
              ProgressView()
              Spacer()
            }
            .listRowBackground(theme.secondaryBackgroundColor)
            .listRowSeparator(.hidden)
            .id(UUID())
          }
        } else if viewModel.allSectionsEmpty {
          EmptyView(iconName: "magnifyingglass",
                    title: "explore.search.title",
                    message: "warpnet.explore.empty")
            .listRowBackground(theme.secondaryBackgroundColor)
            .listRowSeparator(.hidden)
        } else {
          if !viewModel.suggestedAccounts.isEmpty {
            suggestedAccountsSection
          }
        }
      }
      .environment(\.defaultMinListRowHeight, .scrollToViewHeight)
      .task {
        viewModel.client = client
        await viewModel.fetchTrending()
      }
      .refreshable {
        Task {
          HapticManager.shared.fireHaptic(.dataRefresh(intensity: 0.3))
          await viewModel.fetchTrending()
          HapticManager.shared.fireHaptic(.dataRefresh(intensity: 0.7))
        }
      }
      .listStyle(.plain)
      .scrollContentBackground(.hidden)
      .background(theme.secondaryBackgroundColor)
      .navigationTitle("explore.navigation-title")
      .searchable(text: $viewModel.searchQuery,
                  isPresented: $viewModel.isSearchPresented,
                  placement: .navigationBarDrawer(displayMode: .always),
                  prompt: Text("explore.search.prompt"))
      .task(id: viewModel.searchQuery) {
        await viewModel.search()
      }
      .onChange(of: scrollToTopSignal) {
        if viewModel.scrollToTopVisible {
          viewModel.isSearchPresented.toggle()
        } else {
          withAnimation {
            proxy.scrollTo(ScrollToView.Constants.scrollToTop, anchor: .top)
          }
        }
      }
    }
  }

  private var loadingView: some View {
    ForEach(Status.placeholders()) { status in
      StatusRowView(viewModel: .init(status: status, client: client, routerPath: routerPath))
        .padding(.vertical, 8)
        .redacted(reason: .placeholder)
        .allowsHitTesting(false)
        .listRowBackground(theme.primaryBackgroundColor)
    }
  }

  @ViewBuilder
  private func makeSearchResultsView(results: SearchResults) -> some View {
    if !results.accounts.isEmpty {
      Section("explore.section.users") {
        ForEach(results.accounts) { account in
          if let relationship = results.relationships.first(where: { $0.id == account.id }) {
            AccountsListRow(viewModel: .init(account: account, relationShip: relationship))
              .listRowBackground(theme.primaryBackgroundColor)
          }
        }
      }
    }
  }

  private var suggestedAccountsSection: some View {
    Section("explore.section.suggested-users") {
      ForEach(viewModel.suggestedAccounts
        .prefix(upTo: viewModel.suggestedAccounts.count > 3 ? 3 : viewModel.suggestedAccounts.count))
      { account in
        if let relationship = viewModel.suggestedAccountsRelationShips.first(where: { $0.id == account.id }) {
          AccountsListRow(viewModel: .init(account: account, relationShip: relationship))
            .listRowBackground(theme.primaryBackgroundColor)
        }
      }
      NavigationLink(value: RouterDestination.accountsList(accounts: viewModel.suggestedAccounts)) {
        Text("see-more")
          .foregroundColor(theme.tintColor)
      }
      .listRowBackground(theme.primaryBackgroundColor)
    }
  }

  private var scrollToTopView: some View {
    ScrollToView()
      .frame(height: .scrollToViewHeight)
      .onAppear {
        viewModel.scrollToTopVisible = true
      }
      .onDisappear {
        viewModel.scrollToTopVisible = false
      }
  }
}
