// Modified by Warpnet. See NOTICE.

import DesignSystem
import Env
import Models
import SwiftUI

struct StatusRowContentView: View {
  @Environment(\.redactionReasons) private var reasons
  @Environment(\.isCompact) private var isCompact
  @Environment(\.isStatusFocused) private var isFocused

  @Environment(Theme.self) private var theme

  var viewModel: StatusRowViewModel

  var body: some View {
    StatusRowTextView(viewModel: viewModel)
    if let poll = viewModel.finalStatus.poll {
      StatusPollView(poll: poll, status: viewModel.finalStatus)
    }

    if !reasons.contains(.placeholder),
       !isCompact,
       viewModel.isEmbedLoading || viewModel.embeddedStatus != nil
    {
      if let embeddedStatus = viewModel.embeddedStatus {
        StatusEmbeddedView(status: embeddedStatus,
                           client: viewModel.client,
                           routerPath: viewModel.routerPath)
          .fixedSize(horizontal: false, vertical: true)
          .transition(.opacity)
      } else {
        StatusEmbeddedView(status: Status.placeholder(),
                           client: viewModel.client,
                           routerPath: viewModel.routerPath)
          .fixedSize(horizontal: false, vertical: true)
          .redacted(reason: .placeholder)
          .transition(.opacity)
      }
    }

    if !viewModel.finalStatus.mediaAttachments.isEmpty {
      HStack {
        StatusRowMediaPreviewView(attachments: viewModel.finalStatus.mediaAttachments)
        if theme.statusDisplayStyle == .compact {
          Spacer()
        }
      }
      .accessibilityHidden(isFocused == false)
      .padding(.vertical, 4)
    }
  }
}
