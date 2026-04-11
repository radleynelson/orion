import Foundation
import Security

enum KeychainService {
    private static let service = "com.orion.mobile"

    static func saveToken(_ token: String, for host: String) { save(key: "token-\(host)", value: token) }
    static func getToken(for host: String) -> String? { load(key: "token-\(host)") }

    static func saveConnections(_ connections: [SavedConnection]) {
        guard let data = try? JSONEncoder().encode(connections), let string = String(data: data, encoding: .utf8) else { return }
        save(key: "saved-connections", value: string)
    }

    static func loadConnections() -> [SavedConnection] {
        guard let string = load(key: "saved-connections"), let data = string.data(using: .utf8),
              let connections = try? JSONDecoder().decode([SavedConnection].self, from: data) else { return [] }
        return connections
    }

    private static func save(key: String, value: String) {
        guard let data = value.data(using: .utf8) else { return }
        delete(key: key)
        let query: [String: Any] = [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service,
            kSecAttrAccount as String: key, kSecValueData as String: data, kSecAttrAccessible as String: kSecAttrAccessibleWhenUnlocked]
        SecItemAdd(query as CFDictionary, nil)
    }

    private static func load(key: String) -> String? {
        let query: [String: Any] = [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service,
            kSecAttrAccount as String: key, kSecReturnData as String: true, kSecMatchLimit as String: kSecMatchLimitOne]
        var result: AnyObject?
        guard SecItemCopyMatching(query as CFDictionary, &result) == errSecSuccess, let data = result as? Data else { return nil }
        return String(data: data, encoding: .utf8)
    }

    private static func delete(key: String) {
        SecItemDelete([kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: key] as CFDictionary)
    }
}
