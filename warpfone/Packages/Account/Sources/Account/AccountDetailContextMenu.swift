// Modified by Warpnet. See NOTICE.

import Env
import Models
import Network
import SwiftUI

public struct AccountDetailContextMenu: View {
  @Environment(Client.self) private var client
  @Environment(RouterPath.self) private var routerPath

  @Binding var showBlockConfirmation: Bool

  var viewModel: AccountDetailViewModel

  public var body: some View {
    if let account = viewModel.account {
      Section(account.acct) {
        if !viewModel.isCurrentUser {
          Button {
            routerPath.presentedSheet = .mentionStatusEditor(account: account)
          } label: {
            Label("account.action.mention", systemImage: "at")
          }
          Button {
            Task {
              if let conversation: Conversation = try? await client.post(endpoint: Chats.open(accountId: account.id)) {
                routerPath.navigate(to: .conversationDetail(conversation: conversation))
              }
            }
          } label: {
            Label("account.action.message", systemImage: "envelope")
          }

            #if !targetEnvironment(macCatalyst)
            Divider()
            #endif

          if viewModel.relationship?.blocking == true {
            Button {
              Task {
                do {
                  viewModel.relationship = try await client.post(endpoint: Accounts.unblock(id: account.id))
                } catch { }
              }
            } label: {
              Label("account.action.unblock", systemImage: "person.crop.circle.badge.exclamationmark")
            }
          } else {
            Button {
              showBlockConfirmation.toggle()
            } label: {
              Label("account.action.block", systemImage: "person.crop.circle.badge.xmark")
            }
          }

          if viewModel.relationship?.muting == true {
            Button {
              Task {
                do {
                  viewModel.relationship = try await client.post(endpoint: Accounts.unmute(id: account.id))
                } catch { }
              }
            } label: {
              Label("account.action.unmute", systemImage: "speaker")
            }
          } else {
            Button {
              Task {
                do {
                  viewModel.relationship = try await client.post(endpoint: Accounts.mute(id: account.id))
                } catch { }
              }
            } label: {
              Label("account.action.mute", systemImage: "speaker.slash")
            }
          }

          Button(role: .destructive) {
            routerPath.presentedSheet = .reportAccount(account: account)
          } label: {
            Label("warpnet.report.account-action", systemImage: "exclamationmark.bubble")
          }

            #if !targetEnvironment(macCatalyst)
            Divider()
            #endif
        }

        if let url = account.url {
          ShareLink(item: url, subject: Text(account.safeDisplayName)) {
            Label("account.action.share", systemImage: "square.and.arrow.up")
          }
        }

        #if !targetEnvironment(macCatalyst)
        Divider()
        #endif
      }
    }
  }
}
