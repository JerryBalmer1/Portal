package graph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Load reads an envelope from a file. Optional fields may be absent; the
// three lists are normalised to empty slices so callers never see nil.
func Load(path string) (*Envelope, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	env, err := Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return env, nil
}

// Decode reads an envelope from a reader.
func Decode(r io.Reader) (*Envelope, error) {
	var env Envelope
	dec := json.NewDecoder(r)
	dec.UseNumber()
	if err := dec.Decode(&env); err != nil {
		return nil, err
	}
	env.normalise()
	return &env, nil
}

func (e *Envelope) normalise() {
	if e.Nodes == nil {
		e.Nodes = []Node{}
	}
	if e.Edges == nil {
		e.Edges = []Edge{}
	}
	if e.Findings == nil {
		e.Findings = []Finding{}
	}
}

// Marshal serialises an envelope as indented JSON with a trailing newline.
func Marshal(env *Envelope) ([]byte, error) {
	env.normalise()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(env); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Save writes an envelope to a file, recounting first.
func Save(path string, env *Envelope) error {
	env.Recount()
	b, err := Marshal(env)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
