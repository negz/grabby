package main

import (
	"fmt"
	"log"
	"time"

	"github.com/pivotal-golang/bytefmt"
	"gopkg.in/alecthomas/kingpin.v1"

	"github.com/negz/grabby/grabber"
	"github.com/negz/grabby/nntp"
	"github.com/negz/grabby/nzb"
	"github.com/negz/grabby/postprocess"
	"github.com/negz/grabby/util"
)

const Day time.Duration = time.Hour * 24

func watchGrabber(g grabber.Grabberer) {
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			log.Printf("Downloading at %v (health %.f%%)", bytefmt.ByteSize(uint64(g.Strategy().DownloadRate())), g.Health())
		case <-g.PostProcessable():
			log.Printf("Downloading at %v (health %.f%%)", bytefmt.ByteSize(uint64(g.Strategy().DownloadRate())), g.Health())
			return
		}
	}
}

// This is mostly a horrible test harness for the moment.
func main() {
	var (
		server      = kingpin.Flag("server", "Usenet server hostname").Short('s').Required().String()
		username    = kingpin.Flag("username", "Usenet server username").Short('u').Required().String()
		passfile    = kingpin.Flag("passfile", "Usenet server password file").Short('p').Required().ExistingFile()
		connections = kingpin.Flag("connections", "Usenet max connections").Short('c').Default("1").Int()
		retention   = kingpin.Flag("retention", "Usenet retention in days. 0 for unlimited.").Short('r').Default("0").Int()
		needsgroup  = kingpin.Flag("switchgroup", "Send GROUP before getting article").Short('g').Default("false").Bool()
		nzbfile     = kingpin.Arg("nzb", "NZB file to download").Required().ExistingFile()
		outdir      = kingpin.Arg("outdir", "File to download to").Required().ExistingDir()
	)
	kingpin.Parse()

	n, err := nzb.NewFromFile(*nzbfile)
	if err != nil {
		log.Fatalf("Unable to open NZB file: %v", err)
	}

	o := make([]grabber.ServerOption, 0)
	if *retention > 0 {
		o = append(o, grabber.Retention(time.Duration(*retention)*Day))
	}
	if *needsgroup {
		o = append(o, grabber.MustBeInGroup())
	}

	password, err := util.PasswordFromFile(*passfile)
	if err != nil {
		log.Fatalf("Couldn't read password from %v", *passfile)
	}

	nntpServer, err := nntp.NewServer(*server, 119, *connections, nntp.Credentials(*username, password))
	if err != nil {
		log.Fatalf("Error setting up strategy: %v", err)
	}

	grabServer, err := grabber.NewServer(nntpServer, *server, o...)
	if err != nil {
		log.Fatalf("Error setting up strategy: %v", err)
	}

	grabStrategy, err := grabber.NewStrategy([]grabber.Serverer{grabServer})
	if err != nil {
		log.Fatalf("Error setting up strategy: %v", err)
	}

	g, err := grabber.New(*outdir, grabStrategy, grabber.FromNZB(n))
	if err != nil {
		log.Fatalf("Unable to setup grabber: %v", err)
	}

	g.HandleGrabs()

	// Get the bulk of the files.
	if err := g.GrabAll(); err != nil {
		g.Shutdown(err)
		log.Fatalf("Unable to grab all the things: %v", err)
	}
	watchGrabber(g)

	// Start a postprocessor
	pp := postprocess.New(g.WorkDir())
	pp.AddFiles(g.GrabbedFiles())

	// Start the assemble and repair cycle.
	r := pp.Repairer()
	for !r.Repaired() {
		if r.BlocksNeeded() > 0 {
			grabFiles, err := getMorePar2s(r.BlocksNeeded(), g.Par2Files())
			if err != nil {
				g.Shutdown(err)
				log.Fatalf("Unable to repair files: %v", err)
			}
			for _, f := range grabFiles {
				log.Printf("Grabbing extra par2 file %v", f.Filename())
				g.GrabFile(f)
			}
			watchGrabber(g)
			pp.AddFiles(g.GrabbedFiles())
		}

		log.Printf("Assembling...")
		if err := pp.Assemble(); err != nil {
			log.Fatalf("Unable to assemble files: %v", err)
		}
		log.Printf("Repairing...")
		if err := r.Repair(); err != nil {
			log.Fatalf("Unable to repair files: %v", err)
		}
	}

	if err := g.Shutdown(nil); err != nil {
		log.Fatalf("Unable to shutdown grabber: %v", err)
	}

}

func getMorePar2s(blocksNeeded int, par2Files []grabber.Filer) ([]grabber.Filer, error) {
	blocks := 0
	needFiles := 0
	for _, f := range par2Files {
		blocks += postprocess.Par2BlocksFromFilename(f.Filename())
		needFiles++
		if blocks >= blocksNeeded {
			return par2Files[:needFiles], nil
		}
	}
	return par2Files, fmt.Errorf("insufficient blocks to recover - %v of %v", blocks, blocksNeeded)
}
