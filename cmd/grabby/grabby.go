package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
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

	// Parse NZB file
	n, err := nzb.NewFromFile(*nzbfile)
	if err != nil {
		log.Fatalf("Error parsing NZB: %v", err)
	}

	// Make a slice of things to grab.
	var requests []*nntp.ArticleRequest
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
				requests = append(requests, request)
			}
		}
	}
	log.Printf("Will send %v requests", len(requests))

	// Create grabbers
	req := make(chan *nntp.ArticleRequest, len(requests))
	resp := make(chan *nntp.ArticleResponse, len(requests))
	for i := 0; i < s.MaxSessions; i++ {
		sn, err := nntp.NewSession(s, nntp.Dial)
		if err != nil {
			log.Printf("Failed to create new session for %v: %v", s, err)
			continue
		}
		defer sn.Quit()
		go sn.Grab(req, resp)
	}

	// Do the grabbening.
	startTime := time.Now()
	for _, r := range requests {
		req <- r
	}

	var responses []*nntp.ArticleResponse
	var tb int64 = 0
	for len(requests) > len(responses) {
		select {
		case r := <-resp:
			if r.Error != nil {
				log.Printf("Error getting article %v: %v", r.ID, r.Error)
			}
			log.Printf("Wrote %v for article %v", bytefmt.ByteSize(uint64(r.Bytes)), r.ID)
			responses = append(responses, r)
			tb += r.Bytes
		}
	}

	d := time.Since(startTime)
	rate := float64(tb) / d.Seconds()
	log.Printf(
		"Downloaded %v in %v (%v/s)",
		bytefmt.ByteSize(uint64(tb)), d, bytefmt.ByteSize(uint64(rate)))
}
