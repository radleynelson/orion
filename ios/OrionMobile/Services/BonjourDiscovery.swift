import Foundation
import Network

@Observable
final class BonjourDiscovery {
    var discoveredHosts: [DiscoveredHost] = []
    var isSearching = false
    private var browser: NWBrowser?
    private var connection: NWConnection?

    func startBrowsing() {
        guard !isSearching else { return }
        discoveredHosts = []
        isSearching = true
        let params = NWParameters()
        params.includePeerToPeer = true
        browser = NWBrowser(for: .bonjour(type: "_orion._tcp", domain: nil), using: params)
        browser?.browseResultsChangedHandler = { [weak self] results, _ in
            for result in results { self?.resolve(result) }
        }
        browser?.stateUpdateHandler = { [weak self] state in
            if case .failed = state { DispatchQueue.main.async { self?.isSearching = false } }
        }
        browser?.start(queue: .main)
        DispatchQueue.main.asyncAfter(deadline: .now() + 5) { [weak self] in self?.stopBrowsing() }
    }

    func stopBrowsing() { browser?.cancel(); browser = nil; isSearching = false }

    private func resolve(_ result: NWBrowser.Result) {
        let connection = NWConnection(to: result.endpoint, using: .tcp)
        connection.stateUpdateHandler = { [weak self] state in
            if case .ready = state {
                if let path = connection.currentPath, let endpoint = path.remoteEndpoint, case .hostPort(let host, let port) = endpoint {
                    let hostStr: String
                    switch host {
                    case .ipv4(let addr): hostStr = "\(addr)"
                    case .ipv6(let addr): hostStr = "\(addr)"
                    case .name(let name, _): hostStr = name
                    @unknown default: hostStr = "unknown"
                    }
                    let name: String
                    if case .service(let n, _, _, _) = result.endpoint { name = n } else { name = "Orion" }
                    DispatchQueue.main.async {
                        let discovered = DiscoveredHost(name: name, host: hostStr, port: Int(port.rawValue))
                        if !(self?.discoveredHosts.contains(where: { $0.address == discovered.address }) ?? true) {
                            self?.discoveredHosts.append(discovered)
                        }
                    }
                }
                connection.cancel()
            }
        }
        connection.start(queue: .global())
        self.connection = connection
    }
}
