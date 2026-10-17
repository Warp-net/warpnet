import Foundation

enum WarpnetFormat {
  static func html(fromPlainText text: String) -> String {
    guard !text.isEmpty else { return "" }
    var out = ""
    var remaining = Substring(text)
    while let range = remaining.range(of: #"https?://[^\s<>"]+"#, options: .regularExpression) {
      out += escape(remaining[..<range.lowerBound])
      let (link, trailing) = splitTrailingPunctuation(remaining[range])
      let escapedLink = escape(link)
      out += "<a href=\"\(escapedLink)\">\(escapedLink)</a>" + escape(trailing)
      remaining = remaining[range.upperBound...]
    }
    out += escape(remaining)
    let paragraphs = out
      .replacingOccurrences(of: "\r\n", with: "\n")
      .components(separatedBy: "\n\n")
      .map { $0.replacingOccurrences(of: "\n", with: "<br>") }
    return paragraphs.map { "<p>\($0)</p>" }.joined()
  }

  static func date(fromRFC3339 raw: String?) -> Date? {
    guard let raw, raw.count >= 19 else { return nil }
    let chars = Array(raw)
    func number(_ range: Range<Int>) -> Int? {
      guard range.upperBound <= chars.count else { return nil }
      return Int(String(chars[range]))
    }
    guard let year = number(0 ..< 4), let month = number(5 ..< 7), let day = number(8 ..< 10),
          let hour = number(11 ..< 13), let minute = number(14 ..< 16), let second = number(17 ..< 19)
    else { return nil }

    var index = 19
    var fraction = 0.0
    if index < chars.count, chars[index] == "." {
      index += 1
      let start = index
      while index < chars.count, chars[index].isNumber {
        index += 1
      }
      fraction = Double("0." + String(chars[start ..< index])) ?? 0
    }

    var offset = 0
    if index < chars.count, chars[index] == "+" || chars[index] == "-" {
      guard let hours = number(index + 1 ..< index + 3), let minutes = number(index + 4 ..< index + 6) else {
        return nil
      }
      offset = (hours * 3600 + minutes * 60) * (chars[index] == "-" ? -1 : 1)
    }

    var components = DateComponents()
    components.calendar = Calendar(identifier: .gregorian)
    components.timeZone = TimeZone(secondsFromGMT: 0)
    components.year = year
    components.month = month
    components.day = day
    components.hour = hour
    components.minute = minute
    components.second = second
    guard let date = components.date else { return nil }
    return date.addingTimeInterval(fraction - Double(offset))
  }

  static func serverDate(fromRFC3339 raw: String?) -> String {
    serverDate(date(fromRFC3339: raw) ?? Date(timeIntervalSince1970: 0))
  }

  static func serverDate(_ date: Date) -> String {
    ServerDateFormatter.shared.formatter.string(from: date)
  }

  static func imageURL(userId: String, key: String?) -> String? {
    guard let key, !key.isEmpty, !userId.isEmpty else { return nil }
    return "warpnet://image/\(pathComponent(userId))/\(pathComponent(key))"
  }

  static func videoURL(userId: String, key: String?) -> String? {
    guard let key, !key.isEmpty, !userId.isEmpty else { return nil }
    return "warpnet://video/\(pathComponent(userId))/\(pathComponent(key))"
  }

  static func missingImageURL(userId: String) -> String {
    "warpnet://image/\(pathComponent(userId))/missing.png"
  }

  // A Warpnet tweet or user has no web page; a bridged one keeps its
  // Fediverse URL as the tweet id, and its profile lives at host/@user.
  static func webURL(tweetId: String) -> String? {
    guard let scheme = URL(string: tweetId)?.scheme?.lowercased(), scheme == "https" || scheme == "http" else { return nil }
    return tweetId
  }

  static func webURL(userId: String) -> String? {
    let parts = userId.split(separator: "@")
    guard parts.count == 2, parts[1].contains(".") else { return nil }
    return "https://\(parts[1])/@\(parts[0])"
  }

  static func statusURL(id: String) -> String {
    "warpnet://status/\(pathComponent(id))"
  }

  static func statusId(fromURL raw: String) -> String? {
    guard let url = URL(string: raw), url.scheme == "warpnet", url.host() == "status" else { return nil }
    let parts = url.pathComponents.filter { $0 != "/" }
    return parts.count == 1 ? parts[0] : nil
  }

  static func pathComponent(_ value: String) -> String {
    value.addingPercentEncoding(withAllowedCharacters: .alphanumerics.union(CharacterSet(charactersIn: "-._~"))) ?? value
  }

  private static func escape(_ text: Substring) -> String {
    var out = ""
    for char in text {
      switch char {
      case "&": out += "&amp;"
      case "<": out += "&lt;"
      case ">": out += "&gt;"
      case "\"": out += "&quot;"
      case "'": out += "&#39;"
      default: out.append(char)
      }
    }
    return out
  }

  private static func splitTrailingPunctuation(_ link: Substring) -> (Substring, Substring) {
    var end = link.endIndex
    while end > link.startIndex {
      let previous = link.index(before: end)
      let char = link[previous]
      if ".,!?;:".contains(char) {
        end = previous
      } else if char == ")", !link[..<previous].contains("(") {
        end = previous
      } else {
        break
      }
    }
    return (link[..<end], link[end...])
  }
}

private final class ServerDateFormatter: @unchecked Sendable {
  static let shared = ServerDateFormatter()

  let formatter: DateFormatter

  private init() {
    formatter = DateFormatter()
    formatter.calendar = Calendar(identifier: .iso8601)
    formatter.locale = Locale(identifier: "en_US_POSIX")
    formatter.timeZone = TimeZone(secondsFromGMT: 0)
    formatter.dateFormat = "yyyy-MM-dd'T'HH:mm:ss.SSSXXXXX"
  }
}
