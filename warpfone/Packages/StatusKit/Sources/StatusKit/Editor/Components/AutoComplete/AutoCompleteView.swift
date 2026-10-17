// Modified by Warpnet. See NOTICE.

import DesignSystem
import EmojiText
import Foundation
import SwiftUI
import Models

extension StatusEditor {
  
  @MainActor
  struct AutoCompleteView: View {
    @Environment(Theme.self) var theme
    
    var viewModel: ViewModel

    var body: some View {
      if !viewModel.mentionsSuggestions.isEmpty {
        VStack {
          HStack {
            ScrollView(.horizontal, showsIndicators: false) {
              LazyHStack {
                Self.MentionsView(viewModel: viewModel)
              }
              .padding(.horizontal, .layoutPadding)
            }
            .scrollContentBackground(.hidden)
          }
          .frame(height: 40)
        }
        .background(.thinMaterial)
      }
    }
  }
}
