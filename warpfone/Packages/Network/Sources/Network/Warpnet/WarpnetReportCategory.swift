import Foundation

public enum WarpnetReportCategory: String, CaseIterable, Sendable {
  case spam
  case harassment
  case hate
  case violence
  case sexual
  case childSafety
  case selfHarm
  case privacy
  case weapons

  public var titleKey: String {
    "warpnet.report.category.\(rawValue)"
  }

  var hazardClass: String {
    switch self {
    case .spam: "Non-Violent Crimes"
    case .harassment, .hate: "Hate"
    case .violence: "Violent Crimes"
    case .sexual: "Sex Crimes"
    case .childSafety: "Child Exploitation"
    case .selfHarm: "Self-Harm"
    case .privacy: "Privacy"
    case .weapons: "Indiscriminate Weapons"
    }
  }

  public static func reason(for categories: Set<WarpnetReportCategory>, details: String) -> String {
    var classes: [String] = []
    for category in allCases where categories.contains(category) && !classes.contains(category.hazardClass) {
      classes.append(category.hazardClass)
    }
    let joined = classes.joined(separator: ", ")
    let detail = details.trimmingCharacters(in: .whitespacesAndNewlines)
    return detail.isEmpty ? joined : "\(joined) — \(detail)"
  }
}
