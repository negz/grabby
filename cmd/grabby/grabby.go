package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/negz/grabby/decode/yenc"
	"github.com/negz/grabby/nntp"
	"github.com/negz/grabby/nzb"
	"github.com/negz/grabby/util"
	"github.com/pivotal-golang/bytefmt"
	"gopkg.in/alecthomas/kingpin.v1"
)

func outFile(path, nzb, subject string, segment int) string {
	return filepath.Join(path, fmt.Sprintf("%v.%v.%v", nzb, util.HashString(subject), segment))
}

func queueSegments(grabbers *sync.WaitGroup, nzbfile, out string, req chan<- *nntp.ArticleRequest) {
	n, err := nzb.NewFromFile(nzbfile)
	if err != nil {
		log.Fatalf("Error parsing NZB %v: %v", nzbfile, err)
	}

	for _, file := range n.Files {
		for _, group := range file.Groups {
			for _, segment := range file.Segments {
				fp := outFile(out, n.Filename, file.Subject, segment.Number)
				of, err := os.Create(fp)
				if err != nil {
					log.Fatalf("unable to create output file %v: %v", fp, err)
				}
				defer of.Close()
				req <- &nntp.ArticleRequest{Group: group, ID: segment.ArticleID, WriteTo: yenc.NewDecoder(of)}
			}
		}
	}
	close(req)
	// Wait for grabbers to be done before closing output files.
	grabbers.Wait()
}

func displayProgress(st time.Time, resp <-chan *nntp.ArticleResponse) {
	var tb int64 = 0
	for {
		select {
		case r := <-resp:
			if r.Error != nil {
				log.Printf("Error getting article %v: %v", r.ID, r.Error)
			}
			tb += r.Bytes
		default:
			d := time.Since(st)
			rate := float64(tb) / d.Seconds()
			log.Printf(
				"Downloaded %v (%v/s)",
				bytefmt.ByteSize(uint64(tb)), bytefmt.ByteSize(uint64(rate)))
			time.Sleep(time.Second)
		}
	}
}

// This is mostly a horrible test harness for the moment.
func main() {
	var (
		server      = kingpin.Flag("server", "Usenet server hostname").Short('s').Required().String()
		username    = kingpin.Flag("username", "Usenet server username").Short('u').Required().String()
		connections = kingpin.Flag("connections", "Usenet max connections").Short('c').Required().Int()
		passfile    = kingpin.Flag("passfile", "Usenet server password file").Short('p').Required().ExistingFile()
		nzbfile     = kingpin.Arg("nzb", "NZB file to download").Required().ExistingFile()
		outdir      = kingpin.Arg("outdir", "File to download to").Required().ExistingDir()
	)
	kingpin.Parse()

	password, err := util.PasswordFromFile(*passfile)
	if err != nil {
		log.Fatalf("Couldn't read password from %v", *passfile)
	}

	// Connect to server
	s := nntp.NewServer(*server, 119, false, *username, password, *connections)

	// Do the grabbening
	grabbers := new(sync.WaitGroup)
	req := make(chan *nntp.ArticleRequest, s.MaxSessions)
	resp := make(chan *nntp.ArticleResponse, s.MaxSessions)
	startTime := time.Now()
	for i := 0; i < s.MaxSessions; i++ {
		sn, err := nntp.NewSession(s, nntp.Dial)
		if err != nil {
			log.Printf("Failed to create new session for %v: %v", s, err)
			continue
		}
		defer sn.Quit()
		grabbers.Add(1)
		go sn.Grab(grabbers, req, resp)
	}
	go queueSegments(grabbers, *nzbfile, *outdir, req)
	go displayProgress(startTime, resp)
	grabbers.Wait()
	log.Println("Finished!")
}
