package main

import (
	"fmt"
	"os"

	"github.com/codecrafters-io/bittorrent-starter-go/app/internal/torrent"
)

func main() {

	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("go run main.go <torrent-file>")
		return
	}

	torrent.ParseTorrentFile(os.Args[1])
}
