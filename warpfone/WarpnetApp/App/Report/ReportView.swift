// Modified by Warpnet. See NOTICE.

import DesignSystem
import Env
import Models
import Network
import StatusKit
import SwiftUI

public struct ReportView: View {
  @Environment(\.dismiss) private var dismiss

  @Environment(Theme.self) private var theme
  @Environment(Client.self) private var client

  let status: Status?
  let account: Account
  @State private var categories: Set<WarpnetReportCategory> = []
  @State private var commentText: String = ""
  @State private var isSendingReport: Bool = false
  @State private var isErrorDisplayed: Bool = false

  struct ReportSent: Decodable {
    let id: String
  }

  public init(status: Status) {
    self.status = status
    account = status.account
  }

  public init(account: Account) {
    status = nil
    self.account = account
  }

  private var title: LocalizedStringKey {
    status == nil ? "warpnet.report.account-title" : "report.title"
  }

  public var body: some View {
    NavigationStack {
      Form {
        Section {
          ForEach(WarpnetReportCategory.allCases, id: \.self) { category in
            Button {
              if categories.contains(category) {
                categories.remove(category)
              } else {
                categories.insert(category)
              }
            } label: {
              HStack {
                Text(LocalizedStringKey(category.titleKey))
                  .foregroundStyle(theme.labelColor)
                Spacer()
                if categories.contains(category) {
                  Image(systemName: "checkmark")
                    .foregroundStyle(theme.tintColor)
                }
              }
            }
          }
        } header: {
          Text("warpnet.report.categories")
        } footer: {
          Text("warpnet.report.footer")
        }
        .listRowBackground(theme.primaryBackgroundColor)

        Section {
          TextField("report.comment.placeholder",
                    text: $commentText,
                    axis: .vertical)
        }
        .listRowBackground(theme.primaryBackgroundColor)

        if let status {
          StatusEmbeddedView(status: status, client: .init(server: ""), routerPath: RouterPath())
            .allowsHitTesting(false)
            .listRowBackground(theme.primaryBackgroundColor)
        }
      }
      .navigationTitle(title)
      .navigationBarTitleDisplayMode(.inline)
      .scrollContentBackground(.hidden)
      .background(theme.secondaryBackgroundColor)
      .scrollDismissesKeyboard(.immediately)
      .alert("warpnet.report.failed", isPresented: $isErrorDisplayed) {
        Button("OK") {}
      }
      .toolbar {
        ToolbarItem(placement: .navigationBarTrailing) {
          Button {
            isSendingReport = true
            Task {
              do {
                let _: ReportSent =
                  try await client.post(endpoint: Statuses.report(accountId: account.id,
                                                                  statusId: status?.id,
                                                                  comment: WarpnetReportCategory.reason(for: categories,
                                                                                                        details: commentText)))
                HapticManager.shared.fireHaptic(.notification(.success))
                dismiss()
                isSendingReport = false
              } catch {
                isSendingReport = false
                isErrorDisplayed = true
              }
            }
          } label: {
            if isSendingReport {
              ProgressView()
            } else {
              Text("report.action.send")
            }
          }
          .disabled(categories.isEmpty || isSendingReport)
        }

        CancelToolbarItem()
      }
    }
  }
}
