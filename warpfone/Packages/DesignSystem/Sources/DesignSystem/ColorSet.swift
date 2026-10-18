// Modified by Warpnet. See NOTICE.

import SwiftUI

// Colors follow warpdroid's light, dark and black themes.
public let availableColorsSets: [ColorSetCouple] =
  [.init(light: WarpnetLight(), dark: WarpnetDark()),
   .init(light: WarpnetBlackLight(), dark: WarpnetBlack())]

public protocol ColorSet {
  var name: ColorSetName { get }
  var scheme: ColorScheme { get }
  var tintColor: Color { get set }
  var primaryBackgroundColor: Color { get set }
  var secondaryBackgroundColor: Color { get set }
  var labelColor: Color { get set }
}

public enum ColorScheme: String {
  case dark, light
}

public enum ColorSetName: String {
  case warpnetDark = "Warpnet - Dark"
  case warpnetLight = "Warpnet - Light"
  case warpnetBlack = "Warpnet - Black"
  // The light half of the system black theme, so the pair it belongs to stays known.
  case warpnetBlackLight = "Warpnet Black - Light"
}

public struct ColorSetCouple: Identifiable {
  public var id: String {
    dark.name.rawValue + light.name.rawValue
  }

  public let light: ColorSet
  public let dark: ColorSet
}

public struct WarpnetLight: ColorSet {
  public var name: ColorSetName = .warpnetLight
  public var scheme: ColorScheme = .light
  public var tintColor: Color = .brand
  public var primaryBackgroundColor: Color = .white
  public var secondaryBackgroundColor: Color = .init(hex: 0xEBEFF4)
  public var labelColor: Color = .init(hex: 0x16191F)

  public init() {}
}

public struct WarpnetBlackLight: ColorSet {
  public var name: ColorSetName = .warpnetBlackLight
  public var scheme: ColorScheme = .light
  public var tintColor: Color = .brand
  public var primaryBackgroundColor: Color = .white
  public var secondaryBackgroundColor: Color = .init(hex: 0xEBEFF4)
  public var labelColor: Color = .init(hex: 0x16191F)

  public init() {}
}

public struct WarpnetDark: ColorSet {
  public var name: ColorSetName = .warpnetDark
  public var scheme: ColorScheme = .dark
  public var tintColor: Color = .brand
  public var primaryBackgroundColor: Color = .init(hex: 0x282C37)
  public var secondaryBackgroundColor: Color = .init(hex: 0x16191F)
  public var labelColor: Color = .white

  public init() {}
}

public struct WarpnetBlack: ColorSet {
  public var name: ColorSetName = .warpnetBlack
  public var scheme: ColorScheme = .dark
  public var tintColor: Color = .brand
  public var primaryBackgroundColor: Color = .black
  public var secondaryBackgroundColor: Color = .init(hex: 0x16191F)
  public var labelColor: Color = .white

  public init() {}
}
