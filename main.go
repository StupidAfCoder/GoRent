package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/jackpal/bencode-go"
)

const BLOCKSIZE = 16384

const MAXBACKLOG = 5

const port = 6881

type messageID uint8

const (
	MsgChoke         messageID = 0
	MsgUnchoke       messageID = 1
	MsgInterested    messageID = 2
	MsgNotInterested messageID = 3
	MsgHave          messageID = 4
	MsgBitField      messageID = 5
	MsgRequest       messageID = 6
	MsgPiece         messageID = 7
	MsgCancel        messageID = 8
)

type Message struct {
	ID      messageID
	Payload []byte
}

type trackerRespone struct {
	Interval int    `bencode:"interval"`
	Peers    string `bencode:"peers"`
}

func requestPeers(t *torrentFile, peerID [20]byte, port uint16) ([]Peer, error) {
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

	return Unmarshal([]byte(trackerResp.Peers))
}

func generatePeerID() [20]byte {
	var id [20]byte
	copy(id[:], "-GO0001-123456789012")
	return id
}

func parsePieceMessage(index int, buf []byte, msg *Message) (int, error) {
	if msg.ID != MsgPiece {
		return 0, fmt.Errorf("Expected PIECE message got %s", msg.ID)
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

func parseHaveMessage(msg *Message) (int, error) {
	if msg.ID != MsgHave {
		return 0, fmt.Errorf("Expected HAVE , got ID %d", msg.ID)
	}
	if len(msg.Payload) != 4 {
		return 0, fmt.Errorf("Expected Length got %d", msg.Payload)
	}
	index := int(binary.BigEndian.Uint32(msg.Payload))
	return index, nil
}

func (m *Message) Serialize() []byte {
	if m == nil {
		return make([]byte, 4)
	}
	length := uint32(len(m.Payload) + 1)
	buffer := make([]byte, 4+length)
	binary.BigEndian.PutUint32(buffer[0:4], length)
	buffer[4] = byte(m.ID)
	copy(buffer[5:], m.Payload)
	return buffer
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
	client     Client
	buffer     []byte
	downloaded int
	requested  int
	backlog    int
}

type Client struct {
	Conn     net.Conn
	Choked   bool
	Bitfield Bitfield
	peer     Peer
	peerID   [20]byte
	infoHash [20]byte
}

func (c *Client) Read() (*Message, error) {
	msg, err := ReadMessage(c.Conn)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

func (c *Client) sendRequest(index, begin, length int) error {
	req := formatRequest(index, begin, length)
	_, err := c.Conn.Write(req.Serialize())
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) sendInterested() error {
	msg := Message{ID: MsgInterested}
	_, err := c.Conn.Write(msg.Serialize())
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) sendNotInterested() error {
	msg := Message{ID: MsgNotInterested}
	_, err := c.Conn.Write(msg.Serialize())
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) sendUnchoke() error {
	msg := Message{ID: MsgUnchoke}
	_, err := c.Conn.Write(msg.Serialize())
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) sendHave(index int) error {
	msg := formatHave(index)
	_, err := c.Conn.Write(msg.Serialize())
	if err != nil {
		return err
	}
	return nil
}

func formatHave(index int) *Message {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, uint32(index))
	return &Message{ID: MsgHave, Payload: payload}
}

func formatRequest(index, begin, length int) *Message {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload[0:4], uint32(index))
	binary.BigEndian.PutUint32(payload[4:8], uint32(begin))
	binary.BigEndian.PutUint32(payload[8:12], uint32(length))
	return &Message{ID: MsgRequest, Payload: payload}
}

func completeHandshake(conn net.Conn, peerid [20]byte, infohash [20]byte) (*Handshake, error) {
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	defer conn.SetDeadline(time.Time{})

	request := New(infohash, peerid)
	_, err := conn.Write(request.Serialize())
	if err != nil {
		return nil, err
	}

	response, err := ReadHandShake(conn)
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(response.InfoHash[:], infohash[:]) {
		return nil, fmt.Errorf("expected infohash %x but got %x", response.InfoHash, infohash)
	}

	return response, nil
}

func recieveBitField(conn net.Conn) (Bitfield, error) {
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	defer conn.SetDeadline(time.Time{})

	message, err := ReadMessage(conn)
	if err != nil {
		return nil, err
	}
	if message.ID != MsgBitField {
		err := fmt.Errorf("expected bitfield but got Id %d", message.ID)
		return nil, err
	}

	return message.Payload, nil
}

func NewClient(peer Peer, peerid [20]byte, infohash [20]byte) (*Client, error) {
	conn, err := net.DialTimeout("tcp", peer.String(), 3*time.Second)
	if err != nil {
		return nil, err
	}

	_, err = completeHandshake(conn, peerid, infohash)
	if err != nil {
		conn.Close()
		return nil, err
	}

	bf, err := recieveBitField(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &Client{
		Conn:     conn,
		Choked:   true,
		Bitfield: bf,
		peer:     peer,
		peerID:   peerid,
		infoHash: infohash,
	}, nil
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
	case MsgUnchoke:
		state.client.Choked = false
	case MsgChoke:
		state.client.Choked = true
	case MsgHave:
		index, err := parseHaveMessage(msg)
		if err != nil {
			return err
		}
		state.client.Bitfield.SetPiece(index)
	case MsgPiece:
		n, err := parsePieceMessage(state.index, state.buffer, msg)
		if err != nil {
			return err
		}
		state.downloaded += n
		state.backlog--
	}
	return nil
}

type Torrent struct {
	Peers       []Peer
	PeerID      [20]byte
	InfoHash    [20]byte
	PieceHashes [][20]byte
	PieceLength int
	Length      int
	Name        string
}

func attemptToDownloadPiece(client *Client, pieceW *pieceWork) ([]byte, error) {
	state := pieceProgress{
		index:  pieceW.index,
		client: *client,
		buffer: make([]byte, pieceW.length),
	}

	client.Conn.SetDeadline(time.Now().Add(100 * time.Second))
	defer client.Conn.SetDeadline(time.Time{})

	for state.downloaded < pieceW.length {
		if !state.client.Choked {
			for state.backlog < MAXBACKLOG && state.requested < pieceW.length {
				blockSize := BLOCKSIZE
				if pieceW.length-state.requested < blockSize {
					blockSize = pieceW.length - state.requested
				}

				err := client.sendRequest(pieceW.index, state.requested, blockSize)
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
		return fmt.Errorf("The Hash Check Failed For This Piece", pieceW.index)
	}
	return nil
}

func (t *Torrent) startDownloadWorker(peer Peer, workQueue chan *pieceWork, results chan *pieceResult) {
	client, err := NewClient(peer, t.PeerID, t.InfoHash)
	if err != nil {
		log.Printf("Could Not Hanshake with %s", peer.IP)
		return
	}
	defer client.Conn.Close()
	log.Println("Completed Handshake With IP")

	client.sendUnchoke()
	client.sendInterested()

	for pieceW := range workQueue {
		if !client.Bitfield.CheckPiece(pieceW.index) {
			workQueue <- pieceW
			continue
		}

		buf, err := attemptToDownloadPiece(client, pieceW)
		if err != nil {
			log.Println("Exit", err)
			workQueue <- pieceW
			continue
		}

		err = checkIntergrityForPiece(pieceW, buf)
		if err != nil {
			log.Println(err)
			workQueue <- pieceW
			continue
		}

		err = client.sendHave(pieceW.index)
		if err != nil {
			log.Println(err)
			continue
		}
		results <- &pieceResult{pieceW.index, buf}
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

	for _, peer := range t.Peers {
		go t.startDownloadWorker(peer, workQueue, result)
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

type torrentFile struct {
	Announce    string
	InfoHash    [20]byte
	PieceHashes [][20]byte
	PieceLength int
	Length      int
	Name        string
}

func (tf *torrentFile) toTorrent(peers []Peer, peerID [20]byte) *Torrent {
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

type Peer struct {
	IP   net.IP
	port uint16
}

func (p Peer) String() string {
	return net.JoinHostPort(p.IP.String(), strconv.Itoa(int(p.port)))
}

type Bitfield []byte

func (bt Bitfield) CheckPiece(index int) bool {
	byteIndex := index / 8
	offset := index % 8
	return bt[byteIndex]>>(7-offset)&1 != 0
}

func (bt Bitfield) SetPiece(index int) {
	byteIndex := index / 8
	offset := index % 8
	bt[byteIndex] |= 1 << (7 - offset)
}

type Handshake struct {
	Pstr     string
	InfoHash [20]byte
	PeerID   [20]byte
}

func New(infohash, peerID [20]byte) *Handshake {
	return &Handshake{
		Pstr:     "BitTorrent protocol",
		InfoHash: infohash,
		PeerID:   peerID,
	}
}

func (h *Handshake) Serialize() []byte {
	buffer := make([]byte, len(h.Pstr)+49)
	cursor := 1
	buffer[0] = byte(len(h.Pstr))
	cursor += copy(buffer[cursor:], h.Pstr)
	cursor += copy(buffer[cursor:], make([]byte, 8))
	cursor += copy(buffer[cursor:], h.InfoHash[:])
	cursor += copy(buffer[cursor:], h.PeerID[:])
	return buffer
}

func ReadHandShake(r io.Reader) (*Handshake, error) {
	lengthBuffer := make([]byte, 1)
	_, err := io.ReadFull(r, lengthBuffer)
	if err != nil {
		return nil, err
	}
	pstrlen := int(lengthBuffer[0])
	handshakeBuffer := make([]byte, pstrlen+48)
	_, err = io.ReadFull(r, handshakeBuffer)
	if err != nil {
		return nil, err
	}
	h := Handshake{}
	h.Pstr = string(handshakeBuffer[0:pstrlen])
	cursor := pstrlen
	cursor += 8
	copy(h.InfoHash[:], handshakeBuffer[cursor:cursor+20])
	cursor += 20
	copy(h.PeerID[:], handshakeBuffer[cursor:cursor+20])
	return &h, nil
}

func ReadMessage(r io.Reader) (*Message, error) {
	lengthBuffer := make([]byte, 4)
	_, err := io.ReadFull(r, lengthBuffer)
	if err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lengthBuffer)

	if length == 0 {
		return nil, nil
	}

	messageBuffer := make([]byte, length)
	_, err = io.ReadFull(r, messageBuffer)
	if err != nil {
		return nil, err
	}
	m := Message{
		ID:      messageID(messageBuffer[0]),
		Payload: messageBuffer[1:],
	}
	return &m, nil
}

func Unmarshal(peersBin []byte) ([]Peer, error) {
	const peerSize = 6
	numPeers := len(peersBin) / peerSize
	if len(peersBin)%peerSize != 0 {
		err := fmt.Errorf("MalInformation recieved")
		return nil, err
	}
	peers := make([]Peer, numPeers)
	for i := 0; i < numPeers; i++ {
		offset := i * peerSize
		peers[i].IP = net.IP(peersBin[offset : offset+4])
		peers[i].port = binary.BigEndian.Uint16(peersBin[offset+4 : offset+6])
	}
	return peers, nil
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

func (bto *bencodeTorrent) toTorrentFile() (torrentFile, error) {
	infoHash, err := bto.Info.toInfoHash()
	if err != nil {
		return torrentFile{}, err
	}
	pieceHash, err := bto.Info.toPieceHash()
	if err != nil {
		return torrentFile{}, err
	}
	torFile := torrentFile{
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

func (tf *torrentFile) buildTrackerURL(peerID [20]byte, port uint16) (string, error) {
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

func saveToOs(name string, data []byte) error {
	return os.WriteFile(name, data, 0o644)
}

func main() {
	var inputStream io.Reader

	flag.Parse()
	args := flag.Args()

	if len(args) > 0 {
		// User provided an file as argument
		file, err := os.Open(args[0])
		if err != nil {
			log.Fatal("Could Not Parse File Check If it is a torrent")
		}
		defer file.Close()
		inputStream = file
	} else {
		// Checks If The User used to pipe an file!!
		stat, err := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			log.Fatalf("There was an error while piping check if the output was through an human %s", err)
			os.Exit(1)
		}
		inputStream = os.Stdin
	}

	bencodeData, err := Open(inputStream)
	if err != nil {
		panic(err)
	}
	torrentData, err := bencodeData.toTorrentFile()
	if err != nil {
		panic(err)
	}

	peerID := generatePeerID()
	peers, err := requestPeers(&torrentData, peerID, port)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Number Of Peers %d", len(peers))
	torrent := torrentData.toTorrent(peers, peerID)

	data, err := torrent.Download()
	if err != nil {
		log.Fatal(err)
	}

	err = saveToOs(torrent.Name, data)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("The Torrent Has Been Saved To Your Computer --> ", torrent.Name)
}
