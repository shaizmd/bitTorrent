package torrent

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/codecrafters-io/bittorrent-starter-go/app/internal/bencode"
	"github.com/codecrafters-io/bittorrent-starter-go/app/internal/peer"
)

func ParseTorrentFile(filePath string) error {

	torrentData, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	decoded, _, err := bencode.DecodeBencode(torrentData)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	torrent := decoded.(map[string]interface{})

	// ---------------- TRACKER URL ----------------

	announce := string(torrent["announce"].([]byte))

	// ---------------- INFO HASH ----------------

	infoIndex := bytes.Index(torrentData, []byte("4:info"))

	if infoIndex == -1 {
		fmt.Fprintln(os.Stderr, "Error: info dictionary not found")
		os.Exit(1)
	}

	start := infoIndex + len("4:info")

	_, consumed, err := bencode.DecodeBencode(torrentData[start:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	infoBytes := torrentData[start : start+consumed]

	infoHash := sha1.Sum(infoBytes)

	// ---------------- INFO MAP ----------------

	infoMap := torrent["info"].(map[string]interface{})

	name := string(infoMap["name"].([]byte))
	length := infoMap["length"].(int)
	pieceLength := infoMap["piece length"].(int)

	pieces := infoMap["pieces"].([]byte)

	// Validate pieces length
	if len(pieces)%20 != 0 {
		fmt.Fprintln(os.Stderr, "Error: invalid pieces length")
		os.Exit(1)
	}

	// ---------------- OUTPUT ----------------

	fmt.Println("Tracker URL:", announce)
	fmt.Println("Name:", name)
	fmt.Println("Length:", length)

	fmt.Printf("Info Hash: %x\n", infoHash)

	fmt.Println("Piece Length:", pieceLength)

	fmt.Println("Piece Hashes:")

	for i := 0; i < len(pieces); i += 20 {

		hash := pieces[i : i+20]

		fmt.Printf("%x\n", hash)
	}

	// STEP 2 — Create Peer ID
	peerID := "-GO0001-123456789012"

	// STEP 5 — Parse Announce URL
	trackerURL, err := url.Parse(announce)
	if err != nil {
		panic(err)
	}

	// STEP 6 — Add Query Params
	query := trackerURL.Query()
	query.Set("peer_id", peerID)
	query.Set("port", "6881")
	query.Set("uploaded", "0")
	query.Set("downloaded", "0")
	query.Set("left", strconv.Itoa(length))
	query.Set("compact", "1")
	query.Set("info_hash", string(infoHash[:]))

	// STEP 7 — Final URL
	trackerURL.RawQuery = query.Encode()

	fmt.Println("\nContacting tracker...")
	fmt.Println("Tracker Request URL:", trackerURL.String())

	// STEP 8 — Send GET Request
	resp, err := http.Get(trackerURL.String())
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	// STEP 9 — Read Response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	// STEP 10 — Decode Tracker Response
	decodedResp, _, err := bencode.DecodeBencode(body)
	if err != nil {
		panic(err)
	}

	// STEP 11 — Extract Peers
	trackerResp := decodedResp.(map[string]interface{})
	peers := trackerResp["peers"].([]byte)

	fmt.Println("\nPeers discovered:")

	// Parse all peers
	var peerAddresses []string
	for i := 0; i < len(peers); i += 6 {
		ip := fmt.Sprintf(
			"%d.%d.%d.%d",
			peers[i],
			peers[i+1],
			peers[i+2],
			peers[i+3],
		)
		port := binary.BigEndian.Uint16(peers[i+4 : i+6])
		address := fmt.Sprintf("%s:%d", ip, port)
		peerAddresses = append(peerAddresses, address)
		fmt.Println(address)
	}

	// Calculate total number of pieces
	numPieces := len(pieces) / 20
	downloadedPieces := make(map[int][]byte)

	// Try to download all pieces
	for pieceIndex := 0; pieceIndex < numPieces; pieceIndex++ {
		if _, exists := downloadedPieces[pieceIndex]; exists {
			continue // Already downloaded
		}

		pieceHash := pieces[pieceIndex*20 : (pieceIndex+1)*20]

		// Calculate actual piece length (last piece might be smaller)
		currentPieceLength := pieceLength
		if pieceIndex == numPieces-1 {
			// Last piece
			lastPieceLength := length % pieceLength
			if lastPieceLength > 0 {
				currentPieceLength = lastPieceLength
			}
		}

		// Try each peer for this piece
		for _, address := range peerAddresses {
			fmt.Printf("\nTrying to download piece %d from %s\n", pieceIndex, address)

			// Connect to peer
			dialer := net.Dialer{Timeout: 5 * time.Second}
			conn, err := dialer.Dial("tcp", address)
			if err != nil {
				fmt.Printf("Failed to connect to %s: %v\n", address, err)
				continue
			}

			// Send handshake
			peerID := make([]byte, 20)
			_, err = rand.Read(peerID)
			if err != nil {
				conn.Close()
				continue
			}

			handshake := make([]byte, 68)
			handshake[0] = 19
			copy(handshake[1:], "BitTorrent protocol")
			copy(handshake[28:], infoHash[:])
			copy(handshake[48:], peerID)

			_, err = conn.Write(handshake)
			if err != nil {
				fmt.Printf("Failed to send handshake: %v\n", err)
				conn.Close()
				continue
			}

			// Read handshake response
			response := make([]byte, 68)
			_, err = io.ReadFull(conn, response)
			if err != nil {
				fmt.Printf("Failed to read handshake response: %v\n", err)
				conn.Close()
				continue
			}

			// Try to download the piece
			pieceData, err := peer.DownloadPiece(conn, uint32(pieceIndex), currentPieceLength, pieceHash)
			conn.Close()

			if err != nil {
				fmt.Printf("Failed to download piece %d: %v\n", pieceIndex, err)
				continue
			}

			fmt.Printf("Successfully downloaded piece %d: %d bytes\n", pieceIndex, len(pieceData))
			downloadedPieces[pieceIndex] = pieceData
			break // Success, move to next piece
		}

		if _, exists := downloadedPieces[pieceIndex]; !exists {
			fmt.Printf("Warning: Could not download piece %d from any peer\n", pieceIndex)
		}
	}

	// Combine downloaded pieces
	var allPieceData []byte
	for i := 0; i < numPieces; i++ {
		if pieceData, exists := downloadedPieces[i]; exists {
			allPieceData = append(allPieceData, pieceData...)
		}
	}

	// Trim to exact file length (last piece might be padded)
	allPieceData = allPieceData[:length]

	// Save assembled file
	err = os.WriteFile(name, allPieceData, 0644)
	if err != nil {
		fmt.Printf("Failed to save file: %v\n", err)
	} else {
		fmt.Printf("\nSuccessfully downloaded and saved '%s' (%d bytes)\n", name, len(allPieceData))
	}

	return nil
}
