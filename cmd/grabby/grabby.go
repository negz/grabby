package main

import (
	"fmt"
	"github.com/negz/grabby/decode/yenc"
	"github.com/negz/grabby/nntp"
	"github.com/negz/grabby/nzb"
	"github.com/negz/grabby/util"
	"gopkg.in/alecthomas/kingpin.v1"
	"log"
	"os"
	"path/filepath"
	"time"
)

var (
	server      = kingpin.Flag("server", "Usenet server hostname").Short('s').Required().String()
	username    = kingpin.Flag("username", "Usenet server username").Short('u').Required().String()
	connections = kingpin.Flag("connections", "Usenet max connections").Short('c').Required().Int()
	passfile    = kingpin.Flag("passfile", "Usenet server password file").Short('p').Required().ExistingFile()
	nzbfile     = kingpin.Arg("nzb", "NZB file to download").Required().ExistingFile()
	outdir      = kingpin.Arg("outdir", "File to download to").Required().ExistingDir()
)

func outFile(path, nzb, subject string, segment int) string {
	return filepath.Join(path, fmt.Sprintf("%v.%v.%v", nzb, util.HashString(subject), segment))
}

func main() {
	kingpin.Parse()

	password, err := util.PasswordFromFile(*passfile)
	if err != nil {
		log.Fatalf("Couldn't read password from %v", *passfile)
	}

	// Connect to server
	s := nntp.NewServer(*server, 119, false, *username, password, *connections)
	defer s.Disconnect()

	// Parse NZB file
	n, err := nzb.NewFromFile(*nzbfile)
	if err != nil {
		log.Fatalf("Error parsing NZB: %v", err)
	}

	// Make a slice of things to grab.
	grab := make([]*nntp.ArticleRequest, 0)
	for _, file := range n.Files {
		for _, group := range file.Groups {
			for _, segment := range file.Segments {
				fp := outFile(*outdir, n.Filename, file.Subject, segment.Number)
				of, err := os.Create(fp)
				if err != nil {
					log.Fatalf("Unable to create output file %v: %v", fp, err)
				}
				defer of.Close()
				request := &nntp.ArticleRequest{Group: group, ID: segment.ArticleID, WriteTo: yenc.NewDecoder(of)}
				grab = append(grab, request)
			}
		}
	}

	// Send requests.
	// Create grabby channels
	req := make(chan *nntp.ArticleRequest, len(grab))
	resp := make(chan *nntp.ArticleResponse, len(grab))
	for i := 0; i < s.MaxSessions; i++ {
		go s.Grabby(req, resp)
	}
	for _, request := range grab {
		req <- request
	}

	// Wait for the grabbening.
	for {
		select {
		case response := <-resp:
			if response.Error != nil {
				log.Printf("Error getting article %v: %v", response.ID, response.Error)
			}
			log.Printf("Wrote %v bytes for article %v", response.Bytes, response.ID)
		// TODO(negz): This doesn't seem to work.
		case <-time.After(time.Second * 30):
			s.Disconnect()
			log.Fatalln("Suspiciously quiet. Are we done?")
		default:
			log.Println("No responses to handle.")
			time.Sleep(time.Second * 3)
		}
	}

}
