import Foundation

struct LRUCache<Key: Hashable, Value> {
  private struct Entry {
    let value: Value
    let cost: Int
    let storedAt: Date
  }

  private var entries: [Key: Entry] = [:]
  private var order: [Key] = []
  private var totalCost = 0
  private let maxCost: Int
  private let ttl: TimeInterval?

  init(maxCost: Int, ttl: TimeInterval? = nil) {
    self.maxCost = maxCost
    self.ttl = ttl
  }

  mutating func value(for key: Key) -> Value? {
    guard let entry = entries[key] else { return nil }
    if let ttl, Date().timeIntervalSince(entry.storedAt) > ttl {
      remove(key)
      return nil
    }
    touch(key)
    return entry.value
  }

  mutating func set(_ value: Value, for key: Key, cost: Int = 1) {
    remove(key)
    entries[key] = Entry(value: value, cost: cost, storedAt: Date())
    order.append(key)
    totalCost += cost
    while totalCost > maxCost, let oldest = order.first {
      remove(oldest)
    }
  }

  mutating func remove(_ key: Key) {
    guard let entry = entries.removeValue(forKey: key) else { return }
    totalCost -= entry.cost
    if let index = order.firstIndex(of: key) {
      order.remove(at: index)
    }
  }

  private mutating func touch(_ key: Key) {
    if let index = order.firstIndex(of: key) {
      order.remove(at: index)
      order.append(key)
    }
  }
}
