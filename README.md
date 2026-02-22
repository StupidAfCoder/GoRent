# BitTorrent Client in Go

A BitTorrent client written from scratch in Go. It connects to trackers, discovers peers, and downloads files concurrently using the BitTorrent protocol.

> ⚠️ **Note:** This client currently only supports **HTTP/HTTPS** trackers. Torrents that use **UDP** trackers are not supported and will result in an error.

---

## Demo

<!-- Add demo video link here -->
> 🎥 Demo video coming soon...

---

## Project Structure

```
bit-torrent-go/
├── main.go                  # Entry point
├── message/
│   └── message.go           # BitTorrent message types, serialization, parsing
├── peer/
│   └── peer.go              # Peer/client connections, handshake, bitfield
├── torrent/
│   └── torrent.go           # Torrent file parsing, tracker requests, download logic
├── helpers/
│   └── bitfield/
│       └── bitfield.go      # Bitfield type for tracking available pieces
└── test/
    └── debian-13.3.0-amd64-netinst.iso.torrent   # Sample torrent for testing
```

---

## Requirements

- [Go](https://go.dev/dl/) 1.21 or later

---

## Installation

```bash
git clone https://github.com/your-username/bit-torrent-go.git
cd bit-torrent-go
go build -o bittorrent .
```

---

## Usage

### Pass a torrent file as an argument

```bash
./bittorrent path/to/file.torrent
```

### Pipe a torrent file via stdin

```bash
cat path/to/file.torrent | ./bittorrent
```

---

## Testing

A sample `.torrent` file is included in the `test/` folder for quick testing.

```bash
./bittorrent test/debian-13.3.0-amd64-netinst.iso.torrent
```

This will download the **Debian 13.3.0 AMD64 net-install ISO** into your current directory.

> The Debian torrent uses an HTTP tracker so it works out of the box with this client.

---

## Limitations

- **UDP trackers are not supported.** Only `http://` and `https://` tracker URLs are accepted. If a torrent's announce URL uses the `udp://` scheme, the client will exit with an error.
- Magnet links are not supported; a `.torrent` file is required.
- No resume support — downloads start from scratch each run.

---

## Dependencies

| Package | Purpose |
|---|---|
| [`github.com/jackpal/bencode-go`](https://github.com/jackpal/bencode-go) | Bencode encoding/decoding for `.torrent` files and tracker responses |

---

## License

MIT
