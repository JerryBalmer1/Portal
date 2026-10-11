// Package feed packages a Graph.Node manifest card (manifest.yaml and the
// files of its folder) and pushes it to a feed: an OCI registry as an OCI
// artifact, or a NuGet v3 feed as a .nupkg (docs/design.md 40). The card is
// checked with the lifted Graph.Node validator (graph.ValidateManifest),
// never a copy of its rules. Nothing here writes to disk.
package feed

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// File is one file of a card: its path relative to the card's folder, with
// forward slashes, and its bytes.
type File struct {
	Path string
	Data []byte
}

// Card is a validated card ready to push. Files holds manifest.yaml first,
// then the other files sorted by path. Hash is the card's content hash
// (ContentHash), the same whichever feed it goes to.
type Card struct {
	Manifest graph.Manifest
	Files    []File
	Hash     string
}

// ErrInvalid is a card that cannot be pushed: Problems says why.
var ErrInvalid = errors.New("invalid card")

// InvalidError lists every problem with a card. Problems from the manifest
// are Graph.Node's (code schema, path a pointer into manifest.yaml); a file
// path that cannot be packaged is code file.
type InvalidError struct {
	Problems []graph.Problem
	// Name and Version are the manifest's when it decoded far enough to say.
	Name, Version string
}

func (e *InvalidError) Error() string {
	var s []string
	for _, p := range e.Problems {
		s = append(s, p.String())
	}
	return "invalid card: " + strings.Join(s, "; ")
}

func (e *InvalidError) Unwrap() error { return ErrInvalid }

// segment is one segment of a file path a card may hold.
var segment = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._ -]*$`)

// reserved are paths at a card's root that a .nupkg uses itself.
var reserved = map[string]bool{"[content_types].xml": true, "_rels": true, "package": true}

// NewCard validates manifest (the bytes of manifest.yaml) with Graph.Node and
// the file paths, and returns the card. files must not hold manifest.yaml.
func NewCard(manifest []byte, files []File) (*Card, error) {
	bad := &InvalidError{}
	add := func(code, at, msg string) {
		bad.Problems = append(bad.Problems, graph.Problem{Code: code, Path: at, Message: msg, Severity: graph.SeverityError})
	}
	doc, err := graph.DecodeYAML(manifest)
	if err != nil {
		add("schema", "manifest.yaml:/", "not YAML: "+err.Error())
		return nil, bad
	}
	if m, ok := doc.(map[string]any); ok {
		bad.Name, _ = m["name"].(string)
		bad.Version, _ = m["version"].(string)
	}
	for _, p := range graph.ValidateManifest(doc) {
		if p.Severity != graph.SeverityWarning {
			p.Path = "manifest.yaml:" + p.Path
			bad.Problems = append(bad.Problems, p)
		}
	}
	seen := map[string]bool{graph.ManifestFile: true}
	for i, f := range files {
		at := fmt.Sprintf("files/%d", i)
		if msg := checkPath(f.Path); msg != "" {
			add("file", at, msg)
			continue
		}
		if seen[strings.ToLower(f.Path)] {
			add("file", at, fmt.Sprintf("%q twice (paths are compared without case)", f.Path))
		}
		seen[strings.ToLower(f.Path)] = true
	}
	if len(bad.Problems) > 0 {
		return nil, bad
	}
	var m graph.Manifest
	b, _ := json.Marshal(doc)
	if err := json.Unmarshal(b, &m); err != nil {
		add("schema", "manifest.yaml:/", err.Error())
		return nil, bad
	}
	sorted := append([]File(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	c := &Card{Manifest: m, Files: append([]File{{Path: graph.ManifestFile, Data: manifest}}, sorted...)}
	c.Hash = ContentHash(c.Files)
	return c, nil
}

// checkPath says what is wrong with a card file's path, or "".
func checkPath(p string) string {
	switch {
	case p == "":
		return "a file has no path"
	case strings.Contains(p, `\`):
		return fmt.Sprintf("%q: use forward slashes", p)
	case strings.HasPrefix(p, "/") || path.Clean(p) != p:
		return fmt.Sprintf("%q is not a clean path relative to the card's folder", p)
	case strings.EqualFold(p, graph.ManifestFile):
		return "manifest.yaml is sent as the manifest part, not as a file"
	case strings.HasSuffix(strings.ToLower(p), ".nuspec") && !strings.Contains(p, "/"):
		return fmt.Sprintf("%q: a .nuspec at the card's root is the package's own", p)
	}
	parts := strings.Split(p, "/")
	if reserved[strings.ToLower(parts[0])] {
		return fmt.Sprintf("%q: %s at the card's root is reserved by the .nupkg format", p, parts[0])
	}
	for _, s := range parts {
		if s == "." || s == ".." || !segment.MatchString(s) {
			return fmt.Sprintf("%q: segment %q must match %s", p, s, segment)
		}
	}
	return ""
}

// ContentHash is "sha256:" and the SHA-256 of one line per file, sorted by
// path: the path, a tab, the file's own SHA-256 in hex, a newline. It names
// the card's content whatever feed holds it and however it is packaged.
func ContentHash(files []File) string {
	lines := make([]string, 0, len(files))
	for _, f := range files {
		sum := sha256.Sum256(f.Data)
		lines = append(lines, f.Path+"\t"+hex.EncodeToString(sum[:])+"\n")
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}
