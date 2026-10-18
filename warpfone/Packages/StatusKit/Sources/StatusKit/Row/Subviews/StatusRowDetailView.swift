// Modified by Warpnet. See NOTICE.

import DesignSystem
import Env
import Models
import SwiftUI

@MainActor
struct StatusRowDetailView: View {
  @Environment(StatusDataController.self) private var statusDataController

  var viewModel: StatusRowViewModel

  var body: some View {
    VStack(alignment: .leading, spacing: 12) {
      Divider()
      HStack {
        Group {
          Text(viewModel.status.createdAt.asDate, style: .date) +
            Text("status.summary.at-time") +
            Text(viewModel.status.createdAt.asDate, style: .time)
        }.accessibilityElement(children: .combine)
        Spacer()
      }
      .font(.scaledCaption)
      .foregroundStyle(.secondary)

      if let views = viewModel.finalStatus.viewsCount, views > 0 {
        Divider()
        Label("warpnet.status.views-\(views)", systemImage: "eye")
          .font(.scaledCaption)
          .foregroundStyle(.secondary)
      }

      if let editedAt = viewModel.status.editedAt {
        Divider()
        HStack {
          Text("status.summary.edited-time") +
            Text(editedAt.asDate, style: .date) +
            Text("status.summary.at-time") +
            Text(editedAt.asDate, style: .time)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .font(.scaledCaption)
        .foregroundStyle(.secondary)
      }

      if viewModel.actionsAccountsFetched, statusDataController.favoritesCount > 0 {
        Divider()
        Button {
          viewModel.routerPath.navigate(to: .favoritedBy(id: viewModel.status.id))
        } label: {
          HStack {
            Text("status.summary.n-favorites \(statusDataController.favoritesCount)")
              .font(.scaledCallout)
            Spacer()
            makeAccountsScrollView(accounts: viewModel.favoriters)
            Image(systemName: "chevron.right")
          }
          .frame(height: 20)
        }
        .buttonStyle(.borderless)
        .transition(.move(edge: .leading))
      }

      if viewModel.actionsAccountsFetched, statusDataController.reblogsCount > 0 {
        Divider()
        Button {
          viewModel.routerPath.navigate(to: .rebloggedBy(id: viewModel.status.id))
        } label: {
          HStack {
            Text("status.summary.n-boosts \(statusDataController.reblogsCount)")
              .font(.scaledCallout)
            Spacer()
            makeAccountsScrollView(accounts: viewModel.rebloggers)
            Image(systemName: "chevron.right")
          }
          .frame(height: 20)
        }
        .buttonStyle(.borderless)
        .transition(.move(edge: .leading))
      }
    }
    .task {
      await viewModel.fetchActionsAccounts()
    }
  }

  private func makeAccountsScrollView(accounts: [Account]) -> some View {
    ScrollView(.horizontal, showsIndicators: false) {
      LazyHStack(spacing: 0) {
        ForEach(accounts) { account in
          AvatarView(account.avatar, config: .list)
            .padding(.leading, -4)
        }
        .transition(.opacity)
      }
      .padding(.leading, .layoutPadding)
    }
  }
}
