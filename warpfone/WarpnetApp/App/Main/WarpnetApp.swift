// Modified by Warpnet. See NOTICE.

import Account
import AppAccount
import AVFoundation
import DesignSystem
import Env
import MediaUI
import Network
import StatusKit
import SwiftUI
import Timeline

@main
struct WarpnetApp: App {
  @UIApplicationDelegateAdaptor private var appDelegate: AppDelegate

  @Environment(\.scenePhase) var scenePhase
  @Environment(\.openWindow) var openWindow

  @State var appAccountsManager = AppAccountsManager.shared
  @State var currentInstance = CurrentInstance.shared
  @State var currentAccount = CurrentAccount.shared
  @State var userPreferences = UserPreferences.shared
  @State var pairing = PairingModel.shared
  @State var watcher = StreamWatcher.shared
  @State var quickLook = QuickLook.shared
  @State var theme = Theme.shared
  
  @State var selectedTab: Tab = .timeline
  @State var appRouterPath = RouterPath()

  var body: some Scene {
    appScene
    otherScenes
  }

  func setNewClientsInEnv(client: Client) {
    WarpnetDataLoader.shared.setClient(client)
    WarpnetVideoLoader.shared.setClient(client)
    currentAccount.setClient(client: client)
    currentInstance.setClient(client: client)
    userPreferences.setClient(client: client)
    if client.isAuth {
      BackgroundRefresh.requestAuthorization()
    }
    Task {
      await currentInstance.fetchCurrentInstance()
      watcher.setClient(client: client, instanceStreamingURL: currentInstance.instance?.urls?.streamingApi)
      watcher.watch(streams: [.user, .direct])
      await SharedPostPublisher.publish(with: client)
    }
  }

  func handleScenePhase(scenePhase: ScenePhase) {
    switch scenePhase {
    case .background:
      watcher.stopWatching()
      BackgroundRefresh.schedule()
      Task {
        await pairing.pause()
      }
    case .active:
      Task {
        await pairing.resume()
        watcher.watch(streams: [.user, .direct])
        await SharedPostPublisher.publish(with: appAccountsManager.currentClient)
      }
    default:
      break
    }
  }
}

class AppDelegate: NSObject, UIApplicationDelegate {
  func application(_: UIApplication,
                   didFinishLaunchingWithOptions _: [UIApplication.LaunchOptionsKey: Any]? = nil) -> Bool
  {
    try? AVAudioSession.sharedInstance().setCategory(.ambient, options: .mixWithOthers)
    try? AVAudioSession.sharedInstance().setActive(true)
    WarpnetDataLoader.install()
    WarpnetVideoLoader.install()
    BackgroundRefresh.register()
    return true
  }

  func application(_: UIApplication, configurationForConnecting connectingSceneSession: UISceneSession, options _: UIScene.ConnectionOptions) -> UISceneConfiguration {
    let configuration = UISceneConfiguration(name: nil, sessionRole: connectingSceneSession.role)
    if connectingSceneSession.role == .windowApplication {
      configuration.delegateClass = SceneDelegate.self
    }
    return configuration
  }
}
