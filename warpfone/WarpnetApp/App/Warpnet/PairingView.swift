import DesignSystem
import Env
import SwiftUI
import WarpnetTransport

@MainActor
struct PairingView: View {
  @Environment(PairingModel.self) private var pairing
  @Environment(Theme.self) private var theme

  @State private var isManualEntryPresented = false
  @State private var manualPayload = ""

  var body: some View {
    NavigationStack {
      content
        .navigationTitle("warpnet.pairing.title")
        .navigationBarTitleDisplayMode(.inline)
      #if !os(visionOS)
        .scrollContentBackground(.hidden)
        .background(theme.secondaryBackgroundColor)
      #endif
    }
    .sheet(isPresented: $isManualEntryPresented) {
      manualEntryView
    }
  }

  @ViewBuilder
  private var content: some View {
    switch pairing.phase {
    case .restoring, .pairing:
      StartupProgressView()
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    case let .confirming(info, _):
      confirmationView(info)
    case let .failed(message, canRetry):
      failureView(message, canRetry: canRetry)
    case .scanning:
      scanningView
    }
  }

  private var scanningView: some View {
    Form {
      if let notice = pairing.notice {
        Section {
          Text(notice)
            .foregroundStyle(.red)
        }
        #if !os(visionOS)
        .listRowBackground(theme.primaryBackgroundColor)
        #endif
      }
      if pairing.isAvailable {
        Section {
          Text("warpnet.pairing.step-1")
          Text("warpnet.pairing.step-2")
          Text("warpnet.pairing.step-3")
        } header: {
          Text("warpnet.pairing.onboarding-title")
        }
        #if !os(visionOS)
        .listRowBackground(theme.primaryBackgroundColor)
        #endif
        Section {
          if QRScannerView.isAvailable {
            QRScannerView { pairing.scanned($0) }
              .frame(height: 320)
              .listRowInsets(EdgeInsets())
          } else {
            Text("warpnet.pairing.camera-unavailable")
              .foregroundStyle(.secondary)
          }
        }
        #if !os(visionOS)
        .listRowBackground(theme.primaryBackgroundColor)
        #endif
        Section {
          Button("warpnet.pairing.manual") {
            isManualEntryPresented = true
          }
        }
        #if !os(visionOS)
        .listRowBackground(theme.primaryBackgroundColor)
        #endif
      } else {
        Section {
          Text("warpnet.pairing.unavailable")
        }
        #if !os(visionOS)
        .listRowBackground(theme.primaryBackgroundColor)
        #endif
      }
    }
    .formStyle(.grouped)
  }

  private func confirmationView(_ info: AuthNodeInfo) -> some View {
    Form {
      Section {
        LabeledContent("warpnet.pairing.owner", value: info.userId)
        LabeledContent("warpnet.pairing.node", value: shortNodeId(info.nodeId))
        LabeledContent("warpnet.pairing.address",
                       value: info.dialCandidates.first?.replacingOccurrences(of: "/p2p/\(info.nodeId)", with: "") ?? "")
      } header: {
        Text("warpnet.pairing.confirm-title")
      }
      #if !os(visionOS)
      .listRowBackground(theme.primaryBackgroundColor)
      #endif
      Section {
        Button {
          Task {
            await pairing.confirm()
          }
        } label: {
          Text("warpnet.pairing.connect")
            .font(.scaledHeadline)
            .frame(maxWidth: .infinity)
        }
        .buttonStyle(.borderedProminent)
        Button("action.cancel", role: .cancel) {
          pairing.cancel()
        }
        .frame(maxWidth: .infinity)
      }
      #if !os(visionOS)
      .listRowBackground(theme.primaryBackgroundColor)
      #endif
    }
    .formStyle(.grouped)
  }

  private func failureView(_ message: String, canRetry: Bool) -> some View {
    Form {
      Section {
        Text(message)
      }
      #if !os(visionOS)
      .listRowBackground(theme.primaryBackgroundColor)
      #endif
      Section {
        if canRetry {
          Button("warpnet.pairing.retry") {
            Task {
              await pairing.restore()
            }
          }
        }
        Button("warpnet.pairing.scan-new") {
          pairing.cancel()
        }
      }
      #if !os(visionOS)
      .listRowBackground(theme.primaryBackgroundColor)
      #endif
    }
    .formStyle(.grouped)
  }

  private var manualEntryView: some View {
    NavigationStack {
      Form {
        Section {
          TextField("warpnet.pairing.manual-hint", text: $manualPayload, axis: .vertical)
            .lineLimit(4 ... 10)
            .textInputAutocapitalization(.never)
            .autocorrectionDisabled()
          PasteButton(payloadType: String.self) { strings in
            Task { @MainActor in
              manualPayload = strings.first ?? ""
            }
          }
        } footer: {
          Text("warpnet.pairing.manual-body")
        }
        #if !os(visionOS)
        .listRowBackground(theme.primaryBackgroundColor)
        #endif
      }
      .formStyle(.grouped)
      .navigationTitle("warpnet.pairing.manual-title")
      .navigationBarTitleDisplayMode(.inline)
      #if !os(visionOS)
      .scrollContentBackground(.hidden)
      .background(theme.secondaryBackgroundColor)
      #endif
      .toolbar {
        CancelToolbarItem()
        ToolbarItem(placement: .confirmationAction) {
          Button("warpnet.pairing.manual-submit") {
            isManualEntryPresented = false
            pairing.scanned(manualPayload)
            manualPayload = ""
          }
          .disabled(manualPayload.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
        }
      }
    }
  }

  private func shortNodeId(_ nodeId: String) -> String {
    nodeId.count > 16 ? "\(nodeId.prefix(8))…\(nodeId.suffix(6))" : nodeId
  }
}
