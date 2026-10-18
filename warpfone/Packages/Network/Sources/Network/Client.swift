// Modified by Warpnet. See NOTICE.

import Combine
import Foundation
import Models
import Observation
import os
import SwiftUI
import OSLog
import WarpnetTransport

@Observable public final class Client: Equatable, Identifiable, Hashable, @unchecked Sendable {
  public static func == (lhs: Client, rhs: Client) -> Bool {
    lhs.id == rhs.id
  }

  public enum ClientError: Error {
    case unexpectedRequest
    case notPaired
  }

  public var id: String {
    "\(isAuth)\(server)"
  }

  public func hash(into hasher: inout Hasher) {
    hasher.combine(id)
  }

  public let server: String
  public let session: WarpnetSession?
  private let router: WarpnetRouter?
  private let decoder = JSONDecoder()

  private let logger = Logger(subsystem: "site.warpnet.warpfone", category: "networking")

  public var isAuth: Bool {
    session != nil
  }

  public init(server: String, session: WarpnetSession? = nil) {
    self.server = server
    self.session = session
    router = session.map { WarpnetRouter(session: $0) }
    decoder.keyDecodingStrategy = .convertFromSnakeCase
  }

  public func get<Entity: Decodable>(endpoint: Endpoint) async throws -> Entity {
    try await makeEntityRequest(endpoint: endpoint, method: .get)
  }

  public func getWithLink<Entity: Decodable>(endpoint: Endpoint) async throws -> (Entity, LinkHandler?) {
    let response = try await route(endpoint, method: .get)
    let linkHandler = response.next.map { LinkHandler(maxId: $0) }
    return try (decode(Entity.self, from: response.body, endpoint: endpoint), linkHandler)
  }

  public func post<Entity: Decodable>(endpoint: Endpoint) async throws -> Entity {
    try await makeEntityRequest(endpoint: endpoint, method: .post)
  }

  public func post(endpoint: Endpoint) async throws -> HTTPURLResponse? {
    _ = try await route(endpoint, method: .post)
    return okResponse()
  }

  public func patch(endpoint: Endpoint) async throws -> HTTPURLResponse? {
    _ = try await route(endpoint, method: .patch)
    return okResponse()
  }

  public func put<Entity: Decodable>(endpoint: Endpoint) async throws -> Entity {
    try await makeEntityRequest(endpoint: endpoint, method: .put)
  }

  public func delete(endpoint: Endpoint) async throws -> HTTPURLResponse? {
    _ = try await route(endpoint, method: .delete)
    return okResponse()
  }

  public func mediaUpload<Entity: Decodable>(endpoint: Endpoint,
                                             mimeType: String,
                                             filename: String,
                                             data: Data) async throws -> Entity
  {
    guard let router else { throw ClientError.notPaired }
    let body = try await router.upload(endpoint, mimeType: mimeType, filename: filename, data: data)
    return try decode(Entity.self, from: body, endpoint: endpoint)
  }

  public func mediaUpload(endpoint: Endpoint,
                          mimeType: String,
                          filename: String,
                          data: Data) async throws -> HTTPURLResponse?
  {
    guard let router else { throw ClientError.notPaired }
    _ = try await router.upload(endpoint, mimeType: mimeType, filename: filename, data: data)
    return okResponse()
  }

  public func pushes(after lastSeenId: String? = nil) async throws -> [Models.Notification] {
    guard let router else { throw ClientError.notPaired }
    return try await decoder.decode([Models.Notification].self, from: router.pushes(after: lastSeenId))
  }

  public func mediaData(for url: URL) async throws -> Data {
    guard let router else { throw ClientError.notPaired }
    return try await router.mediaData(for: url)
  }

  private func makeEntityRequest<Entity: Decodable>(endpoint: Endpoint, method: WarpnetRouter.Method) async throws -> Entity {
    let response = try await route(endpoint, method: method)
    return try decode(Entity.self, from: response.body, endpoint: endpoint)
  }

  private func route(_ endpoint: Endpoint, method: WarpnetRouter.Method) async throws -> WarpnetRouter.Response {
    guard let router else { throw ClientError.notPaired }
    do {
      return try await router.route(endpoint, method: method)
    } catch {
      logger.error("\(method.rawValue, privacy: .public) \(endpoint.path(), privacy: .public): \(error.localizedDescription, privacy: .public)")
      throw error
    }
  }

  private func decode<Entity: Decodable>(_ type: Entity.Type, from data: Data, endpoint: Endpoint) throws -> Entity {
    do {
      return try decoder.decode(type, from: data)
    } catch {
      logger.error("decoding \(endpoint.path(), privacy: .public): \(error.localizedDescription, privacy: .public)")
      throw error
    }
  }

  private func okResponse() -> HTTPURLResponse? {
    HTTPURLResponse(url: URL(string: "warpnet://node")!, statusCode: 200, httpVersion: nil, headerFields: nil)
  }
}
