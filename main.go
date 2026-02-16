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
	"net/url"
	"os"
	"strconv"

	"github.com/jackpal/bencode-go"
)

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

type Peer struct {
	IP   net.IP
	port uint16
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
	err := bencode.Marshal(&buffer, i)
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
		return nil, fmt.Errorf("Invalid Length Of Pieces %d", len(data))
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

func (t *torrentFile) buildTrackerURL(peerID [20]byte, port uint16) (string, error) {
	base, err := url.Parse(t.Announce)
	if err != nil {
		return "", err
	}
	params := url.Values{
		"info_hash":  []string{string(t.InfoHash[:])},
		"peer_id":    []string{string(peerID[:])},
		"port":       []string{strconv.Itoa(int(port))},
		"uploaded":   []string{"0"},
		"downloaded": []string{"0"},
		"compact":    []string{"1"},
		"left":       []string{strconv.Itoa(t.Length)},
	}
	base.RawQuery = params.Encode()
	return base.String(), nil
}

func main() {
	var inputStream io.Reader

	flag.Parse()
	args := flag.Args()

	if len(args) > 0 {
		//User provided an file as argument
		file, err := os.Open(args[0])
		if err != nil {
			log.Fatal("Could Not Parse File Check If it is a torrent")
		}
		defer file.Close()
		inputStream = file
	} else {
		//Checks If The User used to pipe an file!!
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
	fmt.Printf("Announce URL of the torrent --> %s \n", torrentData.Announce)
}
