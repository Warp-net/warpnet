// Modified by Warpnet. See NOTICE.

import Account
import AppAccount
import DesignSystem
import Env
import Models
import Network
import SwiftUI

struct AccountSettingsView: View {
  @Environment(\.dismiss) private var dismiss
  @Environment(\.openURL) private var openURL

  @Environment(PairingModel.self) private var pairing
  @Environment(CurrentAccount.self) private var currentAccount
  @Environment(Theme.self) private var theme
  @Environment(AppAccountsManager.self) private var appAccountsManager
  @Environment(Client.self) private var client

  @State private var isEditingAccount: Bool = false
  @State private var isEditingFilters: Bool = false

  let account: Account
  let appAccount: AppAccount

  var body: some View {
    Form {
      Section {
        Button {
          isEditingAccount = true
        } label: {
          Label("account.action.edit-info", systemImage: "pencil")
            .frame(maxWidth: .infinity, alignment: .leading)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)

        Button {
          isEditingFilters = true
        } label: {
          Label("account.action.edit-filters", systemImage: "line.3.horizontal.decrease.circle")
            .frame(maxWidth: .infinity, alignment: .leading)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
      }
      .listRowBackground(theme.primaryBackgroundColor)

      Section {
        Button(role: .destructive) {
          Task {
            await pairing.unpair()
            dismiss()
          }
        } label: {
          Text("warpnet.pairing.unpair")
            .frame(maxWidth: .infinity)
        }
      }
      .listRowBackground(theme.primaryBackgroundColor)
    }
    .sheet(isPresented: $isEditingAccount, content: {
      EditAccountView()
    })
    .sheet(isPresented: $isEditingFilters, content: {
      FiltersListView()
    })
    .toolbar {
      ToolbarItem(placement: .principal) {
        HStack {
          AvatarView(account.avatar, config: .embed)
          Text(account.safeDisplayName)
            .font(.headline)
        }
      }
    }
    .navigationTitle(account.safeDisplayName)
    .scrollContentBackground(.hidden)
    .background(theme.secondaryBackgroundColor)
  }
}
