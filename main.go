package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"bitTorrent/torrent"
)

const port = 6881

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

	bencodeData, err := torrent.Open(inputStream)
	if err != nil {
		panic(err)
	}
	torrentData, err := bencodeData.ToTorrentFile()
	if err != nil {
		panic(err)
	}

	peerID := torrent.GeneratePeerID()
	peers, err := torrent.RequestPeers(&torrentData, peerID, port)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Number Of Peers %d", len(peers))
	t := torrentData.ToTorrent(peers, peerID)

	data, err := t.Download()
	if err != nil {
		log.Fatal(err)
	}

	err = saveToOs(t.Name, data)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("The Torrent Has Been Saved To Your Computer --> ", t.Name)
}
