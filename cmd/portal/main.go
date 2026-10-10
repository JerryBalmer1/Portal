// Command portal serves the portal: the one API (api/openapi.yaml) and the
// page that draws the screen graph.
//
//	portal [-port 8200] [-artifacts artifacts] [-tenants tenants]
//	       [-mount name=path]... [-filesystem <dir|file>] [-no-seed]
//
// It serves one tenant, PORTAL_TENANT (default "default"): portal.base.yaml
// (embedded) with <tenants>/<tenant>/portal.yaml merged over it, both
// validated against schema/portal.schema.json, plus that tenant's
// tokens.yaml and connections.yaml when present. A tenant file that fails
// stops the binary (docs/design.md 36).
//
// The screen boots from the newest snap under <artifacts>/snaps, or from the
// one PORTAL_SNAP=<stamp> names, else from the dev seed; the layers its Mount
// nodes record are mounted, then the flags'. PORTAL_MODE=production
// hides every node that is-a Designer. The port is fixed: 0 is refused, and a
// busy port stops the binary rather than moving it (docs/design.md 24).
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
	"time"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/server"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

// defaultFilesystem is where Graph.Emitter.FileSystem writes its layers.
const defaultFilesystem = `C:\__Code\Graph.Emitter.FileSystem\artifacts\layers`

// host is the only interface the portal listens on.
const host = "127.0.0.1"

// DefaultPort is the portal's fixed port; .env.example's PORTAL_PORT.
const DefaultPort = 8200

// checkPort refuses 0 (a port picked at random is not fixed) and anything
// outside 1-65535.
func checkPort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("-port %d: the portal serves on one fixed port, 1-65535 (default %d); 0 is refused", port, DefaultPort)
	}
	return nil
}

type mounts []string

func (m *mounts) String() string     { return strings.Join(*m, ",") }
func (m *mounts) Set(s string) error { *m = append(*m, s); return nil }

func main() {
	port := flag.Int("port", DefaultPort, "TCP port on 127.0.0.1, 1-65535; never picked for you")
	artifacts := flag.String("artifacts", "artifacts", "directory for snaps/; the screen boots from the newest")
	filesystem := flag.String("filesystem", defaultFilesystem, "mounted as layer \"filesystem\" at startup when present (a directory's newest *.graph.json); empty skips it")
	tenants := flag.String("tenants", "tenants", "directory holding <tenant>/portal.yaml (and tokens.yaml, connections.yaml) for PORTAL_TENANT")
	noSeed := flag.Bool("no-seed", false, "with no snap to boot from, start empty instead of seeding the dev screen")
	var ms mounts
	flag.Var(&ms, "mount", "name=path of a layer to mount at startup; repeatable")
	flag.Parse()
	if err := checkPort(*port); err != nil {
		log.Fatalf("portal: %v", err)
	}

	mode := os.Getenv("PORTAL_MODE")
	if mode != "" && mode != "production" && mode != "development" {
		log.Fatalf("portal: PORTAL_MODE is %q; want production, development or unset", mode)
	}
	p, err := store.New(store.Options{Artifacts: *artifacts, Snap: os.Getenv("PORTAL_SNAP"), Production: mode == "production", NoSeed: *noSeed})
	if err != nil {
		log.Fatalf("portal: %v", err)
	}
	fmt.Printf("booted from %s\n", p.Boot())
	t, err := tenant.Load(tenant.Options{Name: os.Getenv("PORTAL_TENANT"), Dir: *tenants, Base: portal.BaseYAML, Schema: portal.ConfigSchema,
		Stamp: time.Now().UTC().Format(store.StampLayout), Check: p.CheckEnvelope})
	if err != nil {
		log.Fatalf("portal: %v", err)
	}
	fmt.Printf("tenant %s from %s (%d menu nodes, %d tokens, %d connections)\n", t.Name, *tenants, len(t.Menu.Nodes), t.Tokens(), len(t.Connections))
	// The screen's Mount nodes first, so a flag naming the same layer wins
	// (docs/design.md 33). A recorded layer that cannot be read is reported
	// and left unmounted; it never stops the boot.
	recorded, failed := p.Remount()
	for _, l := range recorded {
		fmt.Printf("mounted %s from %s (%s, %d nodes, ontology %v)\n", l.Name, l.Path, store.MountID(l.Name), len(l.Env.Nodes), l.Ontology != nil)
	}
	for _, err := range failed {
		fmt.Fprintf(os.Stderr, "portal: not mounted: %v\n", err)
	}
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
		l, err := p.MountAtStart(m)
		if err != nil {
			log.Fatalf("portal: mount %s: %v", m.Name, err)
		}
		fmt.Printf("mounted %s from %s (%d nodes, ontology %v)\n", l.Name, l.Path, len(l.Env.Nodes), l.Ontology != nil)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := server.Serve(ctx, host, *port, server.New(p, t, server.Options{})); err != nil {
		log.Fatalf("portal: port %d: %v; free it, or set PORTAL_PORT in .env or pass -port", *port, err)
	}
}
