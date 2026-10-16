// Modified by Warpnet. See NOTICE.

import Foundation

public enum AppInfo {
  public static let appStoreAppId = "6444915884"
  public static let clientName = "Warpnet"
  public static let scheme = "warpfone://"
  public static let scopes = "read write follow push"
  public static let weblink = "https://github.com/Warp-net/warpnet"
  public static let revenueCatKey = "appl_JXmiRckOzXXTsHKitQiicXCvMQi"
  public static let defaultServer = "mastodon.social"
  public static let appGroup = Bundle.main.object(forInfoDictionaryKey: "AppGroup") as? String
  public static let keychainGroup = Bundle.main.object(forInfoDictionaryKey: "KeychainAccessGroup") as? String
}
