package sdio

import _ "embed"

// EmbeddedTorrent is the default torrent metadata bundled into sdigo.
//
//go:embed seed/SDIO_Update.torrent
var EmbeddedTorrent []byte
