// Modified by Warpnet. See NOTICE.

import DesignSystem
import EmojiText
import Env
import Models
import Network
import StatusKit
import SwiftUI

@MainActor
public struct AccountDetailView: View {
  @Environment(\.openURL) private var openURL
  @Environment(\.redactionReasons) private var reasons
  @Environment(\.openWindow) private var openWindow

  @Environment(StreamWatcher.self) private var watcher
  @Environment(CurrentAccount.self) private var currentAccount
  @Environment(Theme.self) private var theme
  @Environment(Client.self) private var client
  @Environment(RouterPath.self) private var routerPath

  @State private var viewModel: AccountDetailViewModel
  @State private var isCurrentUser: Bool = false
  @State private var showBlockConfirmation: Bool = false

  @State private var isEditingAccount: Bool = false
  @State private var isEditingFilters: Bool = false
  
  @State private var displayTitle: Bool = false

  @Binding var scrollToTopSignal: Int

  /// When coming from a URL like a mention tap in a status.
  public init(accountId: String, scrollToTopSignal: Binding<Int>) {
    _viewModel = .init(initialValue: .init(accountId: accountId))
    _scrollToTopSignal = scrollToTopSignal
  }

  /// When the account is already fetched by the parent caller.
  public init(account: Account, scrollToTopSignal: Binding<Int>) {
    _viewModel = .init(initialValue: .init(account: account))
    _scrollToTopSignal = scrollToTopSignal
  }

  public var body: some View {
    ScrollViewReader { proxy in
      List {
        ScrollToView()
          .onAppear { displayTitle = false }
          .onDisappear { displayTitle = true }
        makeHeaderView(proxy: proxy)
          .applyAccountDetailsRowStyle(theme: theme)
          .padding(.bottom, -20)

        Picker("", selection: $viewModel.selectedTab) {
          ForEach(isCurrentUser ? AccountDetailViewModel.Tab.currentAccountTabs : AccountDetailViewModel.Tab.accountTabs,
                  id: \.self)
          { tab in
            Image(systemName: tab.iconName)
              .tag(tab)
              .accessibilityLabel(tab.accessibilityLabel)
          }
        }
        .pickerStyle(.segmented)
        .padding(.layoutPadding)
        .applyAccountDetailsRowStyle(theme: theme)
        .id("status")

        if viewModel.selectedTab == .statuses {
          pinnedPostsView
        }
        StatusesListView(fetcher: viewModel,
                         client: client,
                         routerPath: routerPath)
      }
      .environment(\.defaultMinListRowHeight, 1)
      .listStyle(.plain)
      .scrollContentBackground(.hidden)
      .background(theme.primaryBackgroundColor)
      .onChange(of: scrollToTopSignal) {
        withAnimation {
          proxy.scrollTo(ScrollToView.Constants.scrollToTop, anchor: .top)
        }
      }
    }
    .onAppear {
      guard reasons != .placeholder else { return }
      isCurrentUser = currentAccount.account?.id == viewModel.accountId
      viewModel.isCurrentUser = isCurrentUser
      viewModel.client = client

      // Avoid capturing non-Sendable `self` just to access the view model.
      let viewModel = viewModel
      Task {
        await withTaskGroup(of: Void.self) { group in
          group.addTask { await viewModel.fetchAccount() }
          group.addTask {
            if await viewModel.statuses.isEmpty {
              await viewModel.fetchNewestStatuses(pullToRefresh: false)
            }
          }
        }
      }
    }
    .refreshable {
      Task {
        HapticManager.shared.fireHaptic(.dataRefresh(intensity: 0.3))
        await viewModel.fetchAccount()
        await viewModel.fetchNewestStatuses(pullToRefresh: true)
        HapticManager.shared.fireHaptic(.dataRefresh(intensity: 0.7))
      }
    }
    .onChange(of: watcher.latestEvent?.id) {
      if let latestEvent = watcher.latestEvent,
         viewModel.accountId == currentAccount.account?.id
      {
        viewModel.handleEvent(event: latestEvent, currentAccount: currentAccount)
      }
    }
    .onChange(of: isEditingAccount) { _, newValue in
      if !newValue {
        Task {
          await viewModel.fetchAccount()
        }
      }
    }
    .sheet(isPresented: $isEditingAccount, content: {
      EditAccountView()
    })
    .sheet(isPresented: $isEditingFilters, content: {
      FiltersListView()
    })
    .edgesIgnoringSafeArea(.top)
    .navigationBarTitleDisplayMode(.inline)
    .toolbar {
      toolbarContent
    }
  }

  @ViewBuilder
  private func makeHeaderView(proxy: ScrollViewProxy?) -> some View {
    switch viewModel.accountState {
    case .loading:
      AccountDetailHeaderView(viewModel: viewModel,
                              account: .placeholder(),
                              scrollViewProxy: proxy)
        .redacted(reason: .placeholder)
        .allowsHitTesting(false)
    case let .data(account):
      AccountDetailHeaderView(viewModel: viewModel,
                              account: account,
                              scrollViewProxy: proxy)
    case let .error(error):
      Text("Error: \(error.localizedDescription)")
    }
  }

  @ViewBuilder
  private var pinnedPostsView: some View {
    if !viewModel.pinned.isEmpty {
      Label("account.post.pinned", systemImage: "pin.fill")
        .accessibilityAddTraits(.isHeader)
        .font(.scaledFootnote)
        .foregroundStyle(.secondary)
        .fontWeight(.semibold)
        .listRowInsets(.init(top: 0,
                             leading: 12,
                             bottom: 0,
                             trailing: .layoutPadding))
        .listRowSeparator(.hidden)
        .listRowBackground(theme.primaryBackgroundColor)
      ForEach(viewModel.pinned) { status in
        StatusRowView(viewModel: .init(status: status, client: client, routerPath: routerPath))
      }
      Rectangle()
        .fill(theme.secondaryBackgroundColor)
        .frame(height: 12)
        .listRowInsets(.init())
        .listRowSeparator(.hidden)
        .accessibilityHidden(true)
    }
  }

  @ToolbarContentBuilder
  private var toolbarContent: some ToolbarContent {
    ToolbarItem(placement: .principal) {
      if let account = viewModel.account, displayTitle {
        VStack {
          Text(account.displayName ?? "").font(.headline)
          Text("account.detail.featured-tags-n-posts \(account.statusesCount ?? 0)")
            .font(.footnote)
            .foregroundStyle(.secondary)
        }
      }
    }
    ToolbarItemGroup(placement: .navigationBarTrailing) {
      if !viewModel.isCurrentUser {
        Button {
          if let account = viewModel.account {
            #if targetEnvironment(macCatalyst)
              openWindow(value: WindowDestinationEditor.mentionStatusEditor(account: account))
            #else
              routerPath.presentedSheet = .mentionStatusEditor(account: account)
            #endif
  
          }
        } label: {
          Image(systemName: "arrowshape.turn.up.left")
        }
      }

      Menu {
        AccountDetailContextMenu(showBlockConfirmation: $showBlockConfirmation, viewModel: viewModel)

        if isCurrentUser {
          Button {
            isEditingAccount = true
          } label: {
            Label("account.action.edit-info", systemImage: "pencil")
          }

          Button {
            isEditingFilters = true
          } label: {
            Label("account.action.edit-filters", systemImage: "line.3.horizontal.decrease.circle")
          }

          Divider()

          Button {
            routerPath.presentedSheet = .settings
          } label: {
            Label("settings.title", systemImage: "gear")
          }
        }
      } label: {
        Image(systemName: "ellipsis.circle")
          .accessibilityLabel("accessibility.tabs.profile.options.label")
          .accessibilityInputLabels([
            LocalizedStringKey("accessibility.tabs.profile.options.label"),
            LocalizedStringKey("accessibility.tabs.profile.options.inputLabel1"),
            LocalizedStringKey("accessibility.tabs.profile.options.inputLabel2"),
          ])
      }
      .confirmationDialog("Block User", isPresented: $showBlockConfirmation) {
        if let account = viewModel.account {
          Button("account.action.block-user-\(account.username)", role: .destructive) {
            Task {
              do {
                viewModel.relationship = try await client.post(endpoint: Accounts.block(id: account.id))
              } catch { }
            }
          }
        }
      } message: {
        Text("account.action.block-user-confirmation")
      }
    }
  }
}

extension View {
  func applyAccountDetailsRowStyle(theme: Theme) -> some View {
    listRowInsets(.init())
      .listRowSeparator(.hidden)
      .listRowBackground(theme.primaryBackgroundColor)
  }
}

struct AccountDetailView_Previews: PreviewProvider {
  static var previews: some View {
    AccountDetailView(account: .placeholder(), scrollToTopSignal: .constant(0))
  }
}
