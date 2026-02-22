package torrent

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/jackpal/bencode-go"

	"bitTorrent/message"
	"bitTorrent/peer"
)

const BLOCKSIZE = 16384

const MAXBACKLOG = 100

var debugLog = log.New(io.Discard, "", 0)

func SetVerbose(v bool) {
	if v {
		debugLog = log.New(os.Stderr, "", log.LstdFlags)
	} else {
		debugLog = log.New(io.Discard, "", 0)
	}
}

type trackerRespone struct {
	Interval int    `bencode:"interval"`
	Peers    string `bencode:"peers"`
}

func RequestPeers(t *TorrentFile, peerID [20]byte, port uint16) ([]peer.Peer, error) {
	urle, err := t.buildTrackerURL(peerID, port)
	if err != nil {
		return nil, err
	}

	annonounceURL, err := url.Parse(t.Announce)
	if err != nil {
		return nil, err
	}

	if annonounceURL.Scheme != "http" && annonounceURL.Scheme != "https" {
		return nil, fmt.Errorf("The URL contains the UDP protocol which is not yet supported! The Protocol is %s", annonounceURL.Scheme)
	}

	resp, err := http.Get(urle)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	trackerResp := trackerRespone{}
	err = bencode.Unmarshal(resp.Body, &trackerResp)
	if err != nil {
		return nil, err
	}

	return peer.Unmarshal([]byte(trackerResp.Peers))
}

func GeneratePeerID() [20]byte {
	var id [20]byte
	copy(id[:], "-GO0001-123456789012")
	return id
}

func parsePieceMessage(index int, buf []byte, msg *message.Message) (int, error) {
	if msg.ID != message.MsgPiece {
		return 0, fmt.Errorf("Expected PIECE message got %x", msg.ID)
	}
	if len(msg.Payload) < 8 {
		return 0, fmt.Errorf("Expected Payload greater than 8 got %d", len(msg.Payload))
	}
	parsedIndex := int(binary.BigEndian.Uint32(msg.Payload[0:4]))
	if parsedIndex != index {
		return 0, fmt.Errorf("Got The Wrong Piece Here Expected %d", index)
	}
	begin := int(binary.BigEndian.Uint32(msg.Payload[4:8]))
	if begin >= len(buf) {
		return 0, fmt.Errorf("Begin is too HIGH")
	}
	data := msg.Payload[8:]
	if len(data)+begin > len(buf) {
		return 0, fmt.Errorf("Data is too long for the offset Begin %d %d", len(data), begin)
	}
	copy(buf[begin:], data)
	return len(data), nil
}

func parseHaveMessage(msg *message.Message) (int, error) {
	if msg.ID != message.MsgHave {
		return 0, fmt.Errorf("Expected HAVE , got ID %d", msg.ID)
	}
	if len(msg.Payload) != 4 {
		return 0, fmt.Errorf("Expected Length got %d", msg.Payload)
	}
	index := int(binary.BigEndian.Uint32(msg.Payload))
	return index, nil
}

type pieceWork struct {
	index  int
	hash   [20]byte
	length int
}

type pieceResult struct {
	index int
	buf   []byte
}

type pieceProgress struct {
	index      int
	client     *peer.Client
	buffer     []byte
	downloaded int
	requested  int
	backlog    int
}

type Torrent struct {
	Peers       []peer.Peer
	PeerID      [20]byte
	InfoHash    [20]byte
	PieceHashes [][20]byte
	PieceLength int
	Length      int
	Name        string
}

func (state *pieceProgress) checkState() error {
	msg, err := state.client.Read()
	if err != nil {
		return err
	}
	if msg == nil {
		return nil
	}
	switch msg.ID {
	case message.MsgUnchoke:
		state.client.Choked = false
	case message.MsgChoke:
		state.client.Choked = true
	case message.MsgHave:
		index, err := parseHaveMessage(msg)
		if err != nil {
			return err
		}
		state.client.Bitfield.SetPiece(index)
	case message.MsgPiece:
		n, err := parsePieceMessage(state.index, state.buffer, msg)
		if err != nil {
			return err
		}
		state.downloaded += n
		state.backlog--
	}
	return nil
}

func attemptToDownloadPiece(client *peer.Client, pieceW *pieceWork) ([]byte, error) {
	state := pieceProgress{
		index:  pieceW.index,
		client: client,
		buffer: make([]byte, pieceW.length),
	}

	client.Conn.SetDeadline(time.Now().Add(30 * time.Second))
	defer client.Conn.SetDeadline(time.Time{})

	for state.downloaded < pieceW.length {
		if !state.client.Choked {
			for state.backlog < MAXBACKLOG && state.requested < pieceW.length {
				blockSize := BLOCKSIZE
				if pieceW.length-state.requested < blockSize {
					blockSize = pieceW.length - state.requested
				}

				err := client.SendRequest(pieceW.index, state.requested, blockSize)
				if err != nil {
					return nil, err
				}

				state.backlog++
				state.requested += blockSize
			}
		}

		err := state.checkState()
		if err != nil {
			return nil, err
		}
	}

	return state.buffer, nil
}

func checkIntergrityForPiece(pieceW *pieceWork, buf []byte) error {
	hash := sha1.Sum(buf)
	if !bytes.Equal(hash[:], pieceW.hash[:]) {
		return fmt.Errorf("The Hash Check Failed For This Piece %d", pieceW.index)
	}
	return nil
}

func (t *Torrent) startDownloadWorker(p peer.Peer, workQueue chan *pieceWork, results chan *pieceResult) {
	backoff := time.Second
	for {
		client, err := peer.NewClient(p, t.PeerID, t.InfoHash)
		if err != nil {
			debugLog.Printf("Could Not Hanshake with %s", p.IP)
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second

		client.SendUnchoke()
		client.SendInterested()

		for pieceW := range workQueue {
			if !client.Bitfield.CheckPiece(pieceW.index) {
				workQueue <- pieceW
				continue
			}

			buf, err := attemptToDownloadPiece(client, pieceW)
			if err != nil {
				debugLog.Println("Peer Disconnected ", err)
				client.Conn.Close()
				workQueue <- pieceW
				break
			}

			err = checkIntergrityForPiece(pieceW, buf)
			if err != nil {
				log.Println(err)
				workQueue <- pieceW
				continue
			}

			client.SendHave(pieceW.index)
			results <- &pieceResult{pieceW.index, buf}
		}
	}
}

func (t *Torrent) calculateBoundsForPiece(index int) (begin int, end int) {
	begin = index * t.PieceLength
	end = begin + t.PieceLength
	if end > t.Length {
		end = t.Length
	}
	return begin, end
}

func (t *Torrent) calculateLengthForPiece(index int) int {
	begin, end := t.calculateBoundsForPiece(index)
	return end - begin
}

func (t *Torrent) Download() ([]byte, error) {
	log.Println("Starting Download For", t.Name)
	workQueue := make(chan *pieceWork, len(t.PieceHashes))
	result := make(chan *pieceResult)
	for index, hash := range t.PieceHashes {
		length := t.calculateLengthForPiece(index)
		workQueue <- &pieceWork{index, hash, length}
	}

	for _, p := range t.Peers {
		go t.startDownloadWorker(p, workQueue, result)
	}

	bud := make([]byte, t.Length)
	donePieces := 0
	for donePieces < len(t.PieceHashes) {
		res := <-result
		begin, end := t.calculateBoundsForPiece(res.index)
		copy(bud[begin:end], res.buf)
		donePieces++

		percent := float64(donePieces) / float64(len(t.PieceHashes)) * 100
		numWorkers := runtime.NumGoroutine() - 1
		fmt.Printf("(%.2f%%) Downloaded Piece %d from %d peers\n", percent, res.index, numWorkers)
	}
	close(workQueue)
	return bud, nil
}

type bencodeInfo struct {
	Pieces      string `bencode:"pieces"`
	PieceLength int    `bencode:"piece length"`
	Length      int    `bencode:"length"`
	Name        string `bencode:"name"`
}

type bencodeTorrent struct {
	Announce string      `bencode:"announce"`
	Info     bencodeInfo `bencode:"info"`
}

type TorrentFile struct {
	Announce    string
	InfoHash    [20]byte
	PieceHashes [][20]byte
	PieceLength int
	Length      int
	Name        string
}

func (tf *TorrentFile) ToTorrent(peers []peer.Peer, peerID [20]byte) *Torrent {
	return &Torrent{
		Peers:       peers,
		PeerID:      peerID,
		InfoHash:    tf.InfoHash,
		PieceHashes: tf.PieceHashes,
		PieceLength: tf.PieceLength,
		Length:      tf.Length,
		Name:        tf.Name,
	}
}

func (i *bencodeInfo) toInfoHash() ([20]byte, error) {
	var buffer bytes.Buffer
	err := bencode.Marshal(&buffer, *i)
	if err != nil {
		log.Fatal(err.Error())
		return [20]byte{}, err
	}
	InfoHash := sha1.Sum(buffer.Bytes())
	return InfoHash, nil
}

func (i *bencodeInfo) toPieceHash() ([][20]byte, error) {
	data := []byte(i.Pieces)
	hashLen := 20

	if len(data)%hashLen != 0 {
		return nil, fmt.Errorf("invalid length of pieces %d", len(data))
	}

	numHashes := len(data) / hashLen
	hashes := make([][20]byte, numHashes)

	for i := 0; i < numHashes; i++ {
		start := i * hashLen
		end := start + hashLen
		copy(hashes[i][:], data[start:end])
	}

	return hashes, nil
}

func (bto *bencodeTorrent) ToTorrentFile() (TorrentFile, error) {
	infoHash, err := bto.Info.toInfoHash()
	if err != nil {
		return TorrentFile{}, err
	}
	pieceHash, err := bto.Info.toPieceHash()
	if err != nil {
		return TorrentFile{}, err
	}
	torFile := TorrentFile{
		Announce:    bto.Announce,
		InfoHash:    infoHash,
		PieceHashes: pieceHash,
		PieceLength: bto.Info.PieceLength,
		Length:      bto.Info.Length,
		Name:        bto.Info.Name,
	}
	return torFile, nil
}

func Open(r io.Reader) (*bencodeTorrent, error) {
	bto := bencodeTorrent{}
	err := bencode.Unmarshal(r, &bto)
	if err != nil {
		fmt.Println("Error")
		return nil, err
	}
	return &bto, nil
}

func percentEncode(b []byte) string {
	res := ""
	for _, v := range b {
		res += fmt.Sprintf("%%%02X", v)
	}
	return res
}

func (tf *TorrentFile) buildTrackerURL(peerID [20]byte, port uint16) (string, error) {
	base, err := url.Parse(tf.Announce)
	if err != nil {
		return "", err
	}
	params := url.Values{
		"port":       []string{strconv.Itoa(int(port))},
		"uploaded":   []string{"0"},
		"downloaded": []string{"0"},
		"compact":    []string{"1"},
		"left":       []string{strconv.Itoa(tf.Length)},
	}
	base.RawQuery = params.Encode()
	base.RawQuery += "&info_hash=" + percentEncode(tf.InfoHash[:])
	base.RawQuery += "&peer_id=" + percentEncode(peerID[:])
	return base.String(), nil
}
