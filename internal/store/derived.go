package store

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// A derived layer (docs/design.md 55) is one Portal builds itself from what
// it holds, the tenant's ledger.jsonl today (tenant-actions), and mounts
// like any other: validated by Graph.Node, in the tenant graph, the roll
// call and the query. It is built at boot and rebuilt whenever what it is
// built from changes, checked on every read of the mounted layers, so a
// line the Runner appends is in it on the next read. It is never written
// anywhere: it is rebuilt from the ledger on every start, which is why it
// survives one. Its name is Portal's: POST .../layers and -mount refuse it.

// Derived names a derived layer and how to build it.
type Derived struct {
	Name string
	// Build is given the other mounted layers. It returns a key naming what
	// the layer would be built from, and the builder; the layer is rebuilt
	// only when the key changes. A builder that returns no envelope
	// unmounts the layer.
	Build func(others []*Layer) (key string, build func() (*graph.Envelope, error))
}

// derivedState is the Portal's derived layer and its last build.
type derivedState struct {
	sync.Mutex
	d   *Derived
	key string
	err error
}

// SetDerived registers d and mounts it now. The error is the first build's.
func (p *Portal) SetDerived(d Derived) error {
	p.derived.Lock()
	p.derived.d, p.derived.key, p.derived.err = &d, "", nil
	p.derived.Unlock()
	p.syncDerived()
	return p.DerivedError()
}

// DerivedError is why the derived layer's last build was not mounted; nil
// when it was. A layer that fails is left as it was last mounted.
func (p *Portal) DerivedError() error {
	p.derived.Lock()
	defer p.derived.Unlock()
	return p.derived.err
}

// derivedName is the derived layer's name, "" when none is registered.
func (p *Portal) derivedName() string {
	p.derived.Lock()
	defer p.derived.Unlock()
	if p.derived.d == nil {
		return ""
	}
	return p.derived.d.Name
}

// unmount removes the layer name; no such layer is nothing to do.
func (p *Portal) unmount(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, l := range p.layers {
		if l.Name == name {
			p.layers = append(p.layers[:i], p.layers[i+1:]...)
			return
		}
	}
}

// syncDerived rebuilds and mounts the derived layer when its key has
// changed. One build at a time; a read that finds a build running waits for
// it, so it sees the layer that build mounts. Never called with mu held.
func (p *Portal) syncDerived() {
	p.derived.Lock()
	defer p.derived.Unlock()
	d := p.derived.d
	if d == nil {
		return
	}
	p.mu.Lock()
	var others []*Layer
	for _, l := range p.layers {
		if l.Name != d.Name {
			others = append(others, l)
		}
	}
	p.mu.Unlock()
	key, build := d.Build(others)
	if key == p.derived.key {
		return
	}
	env, err := build()
	if err == nil && env == nil {
		p.unmount(d.Name)
	} else if err == nil {
		var data []byte
		if data, err = json.Marshal(env); err == nil {
			_, err = p.mountLayer(Mount{Name: d.Name, Envelope: data}, false)
		}
	}
	if err != nil {
		p.derived.err = fmt.Errorf("layer %s: %w", d.Name, err)
		return
	}
	p.derived.key, p.derived.err = key, nil
}
