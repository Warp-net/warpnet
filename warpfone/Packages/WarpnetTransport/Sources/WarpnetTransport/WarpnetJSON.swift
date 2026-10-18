import Foundation

public enum WarpnetJSON {
  public static func encode<T: Encodable>(_ value: T) throws -> String {
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.withoutEscapingSlashes]
    let data = try encoder.encode(value)
    return String(decoding: data, as: UTF8.self)
  }

  public static func decode<T: Decodable>(_ type: T.Type, from raw: String) throws -> T {
    do {
      return try JSONDecoder().decode(type, from: Data(raw.utf8))
    } catch {
      throw WarpnetError.serializationError("\(T.self): \(error)")
    }
  }
}
