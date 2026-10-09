// Command portal serves the portal: the one API (api/openapi.yaml) and the
// page that draws the screen graph.
//
//	portal [-port 8080] [-artifacts artifacts] [-mount name=path]...
//	       [-filesystem <dir|file>] [-no-seed]
//
// The screen boots from the newest snap under <artifacts>/snaps, or from the
// one PORTAL_SNAP=<stamp> names, else from the dev seed. PORTAL_MODE=production
// hides every node that is-a Designer.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/JerryBalmer1/Portal/internal/server"
	"github.com/JerryBalmer1/Portal/internal/store"
)

// defaultFilesystem is where Graph.Emitter.FileSystem writes its layers.
const defaultFilesystem = `C:\__Code\Graph.Emitter.FileSystem\artifacts\layers`

// host is the only interface the portal listens on.
const host = "127.0.0.1"

type mounts []string

func (m *mounts) String() string     { return strings.Join(*m, ",") }
func (m *mounts) Set(s string) error { *m = append(*m, s); return nil }

func main() {
	port := flag.Int("port", 8080, "TCP port on 127.0.0.1; 0 picks a free one")
	artifacts := flag.String("artifacts", "artifacts", "directory for snaps/; the screen boots from the newest")
	filesystem := flag.String("filesystem", defaultFilesystem, "mounted as layer \"filesystem\" at startup when present (a directory's newest *.graph.json); empty skips it")
	noSeed := flag.Bool("no-seed", false, "with no snap to boot from, start empty instead of seeding the dev screen")
	var ms mounts
	flag.Var(&ms, "mount", "name=path of a layer to mount at startup; repeatable")
	flag.Parse()

	mode := os.Getenv("PORTAL_MODE")
	if mode != "" && mode != "production" && mode != "development" {
		log.Fatalf("portal: PORTAL_MODE is %q; want production, development or unset", mode)
	}
	p, err := store.New(store.Options{Artifacts: *artifacts, Snap: os.Getenv("PORTAL_SNAP"), Production: mode == "production", NoSeed: *noSeed})
	if err != nil {
		log.Fatalf("portal: %v", err)
	}
	fmt.Printf("booted from %s\n", p.Boot())
	if *filesystem != "" {
		if _, err := os.Stat(*filesystem); err == nil {
			ms = append([]string{"filesystem=" + *filesystem}, ms...)
		} else {
			fmt.Fprintf(os.Stderr, "portal: %s not present; layer filesystem not mounted\n", *filesystem)
		}
	}
	for _, s := range ms {
		m, err := store.ParseMountFlag(s)
		if err != nil {
			log.Fatalf("portal: %v", err)
		}
		l, err := p.MountLayer(m)
		if err != nil {
			log.Fatalf("portal: mount %s: %v", m.Name, err)
		}
		fmt.Printf("mounted %s from %s (%d nodes, ontology %v)\n", l.Name, l.Path, len(l.Env.Nodes), l.Ontology != nil)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := server.Serve(ctx, host, *port, server.New(p)); err != nil {
		log.Fatal(err)
	}
}
