package main

import (
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
			log.Printf("Downloading at %v", bytefmt.ByteSize(uint64(g.Strategy().DownloadRate())))
		case <-g.PostProcessable():
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

	if err := g.GrabAll(); err != nil {
		log.Fatalf("Unable to grab all the things: %v", err)
	}

	watchGrabber(g)

	pp := postprocess.New(g.WorkDir())
	if err := pp.Assemble(g.GrabbedFiles()); err != nil {
		log.Fatalf("Unable to assemble files: %v", err)
	}
	if err := pp.Repair(); err != nil {
		log.Fatalf("Unable to repair files: %v", err)
	}
	if err := pp.Extract(); err != nil {
		log.Fatalf("Unable to extract files: %v", err)
	}

	if err := g.Shutdown(nil); err != nil {
		log.Fatalf("Unable to shutdown grabber: %v", err)
	}

}
