package graph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"go.yaml.in/yaml/v3"
)

// ManifestFile is the card's file name in each packaged folder; IndexFile is
// the derived index at a repo root (design.md 21).
const (
	ManifestFile = "manifest.yaml"
	IndexFile    = "index.yaml"
)

// Manifest is one card: what a packaged emitter, skill, tool or rule is, what
// it needs and what it does (manifest.schema.json).
type Manifest struct {
	Name        string            `json:"name" yaml:"name"`
	Version     string            `json:"version" yaml:"version"`
	Type        string            `json:"type" yaml:"type"` // emitter | skill | tool | rule
	Description string            `json:"description" yaml:"description"`
	Tags        []string          `json:"tags,omitempty" yaml:"tags,omitempty"`
	Needs       []ManifestNeed    `json:"needs,omitempty" yaml:"needs,omitempty"`
	Verbs       []ManifestVerb    `json:"verbs,omitempty" yaml:"verbs,omitempty"`
	Pins        map[string]string `json:"pins,omitempty" yaml:"pins,omitempty"`
}

// ManifestNeed is one thing a card needs to run.
type ManifestNeed struct {
	Name         string `json:"name" yaml:"name"`
	VersionRange string `json:"version-range,omitempty" yaml:"version-range,omitempty"`
	OS           string `json:"os,omitempty" yaml:"os,omitempty"` // windows | linux | any
}

// ManifestVerb is one thing a card does, with the strongest effect it can have.
type ManifestVerb struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Effect      string `json:"effect" yaml:"effect"` // read | write | destroy
}

// Index is a repo's index.yaml, derived from its cards (index.schema.json).
type Index struct {
	Repo      string       `json:"repo" yaml:"repo"`
	Generated string       `json:"generated" yaml:"generated"`
	Entries   []IndexEntry `json:"entries" yaml:"entries"`
}

// IndexEntry is one card in the index; Path is its folder relative to the
// repo root with forward slashes, "." for the root itself.
type IndexEntry struct {
	Path    string   `json:"path" yaml:"path"`
	Name    string   `json:"name" yaml:"name"`
	Version string   `json:"version" yaml:"version"`
	Type    string   `json:"type" yaml:"type"`
	Tags    []string `json:"tags,omitempty" yaml:"tags,omitempty"`
}

// DecodeYAML reads YAML (or JSON) as plain JSON-shaped data, the form the
// schema validator takes: maps are map[string]any, lists []any.
func DecodeYAML(data []byte) (any, error) {
	var doc any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return normaliseYAML(doc), nil
}

// normaliseYAML turns map[any]any (a mapping with non-string keys) into
// map[string]any so the schema validator sees plain JSON-shaped data.
func normaliseYAML(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			t[k] = normaliseYAML(val)
		}
		return t
	case map[any]any:
		m := make(map[string]any, len(t))
		for k, val := range t {
			m[fmt.Sprint(k)] = normaliseYAML(val)
		}
		return m
	case []any:
		for i, val := range t {
			t[i] = normaliseYAML(val)
		}
		return t
	default:
		return v
	}
}

// ValidateManifest checks a decoded card against manifest.schema.json. Every
// problem is code schema.
func ValidateManifest(doc any) []Problem {
	return finish(validateAgainst("manifest.schema.json", "", doc))
}

// ValidateIndex checks a decoded index against index.schema.json; a generated
// stamp that is not UTC ending Z is code stamp, anything else schema.
func ValidateIndex(doc any) []Problem {
	return finish(validateAgainst("index.schema.json", "", doc))
}

// LoadManifest reads and validates one card. The error is for a file that
// cannot be read; a file that is not YAML is a schema problem. The Manifest is
// nil when there is any error problem.
func LoadManifest(path string) (*Manifest, []Problem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	doc, err := DecodeYAML(data)
	if err != nil {
		return nil, []Problem{{Code: "schema", Path: "/", Message: "not YAML: " + err.Error(), Severity: SeverityError}}, nil
	}
	ps := ValidateManifest(doc)
	if HasErrors(ps) {
		return nil, ps, nil
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return nil, nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, nil, err
	}
	return &m, ps, nil
}

// skipDirs are never walked for cards.
var skipDirs = map[string]bool{".git": true, "node_modules": true}

// BuildIndex walks root for manifest.yaml files (skipping .git and
// node_modules), validates each, and returns the index of the valid ones,
// sorted by path. A problem's path is the card's file relative to root, a
// colon, then the location in the card. Two cards with one name are
// duplicate-name, reported on the later card. The index is complete only when
// HasErrors(problems) is false; the error is for a root that cannot be walked.
func BuildIndex(root string, generated time.Time) (*Index, []Problem, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, err
	}
	if st, err := os.Stat(abs); err != nil {
		return nil, nil, err
	} else if !st.IsDir() {
		return nil, nil, fmt.Errorf("%s is not a directory", root)
	}
	var cards []string
	err = filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && p != abs && skipDirs[d.Name()] {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == ManifestFile {
			cards = append(cards, p)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(cards)
	idx := &Index{Repo: filepath.Base(abs), Generated: generated.UTC().Format("2006-01-02T15:04:05Z"), Entries: []IndexEntry{}}
	var problems []Problem
	owner := map[string]string{}
	for _, card := range cards {
		rel, err := filepath.Rel(abs, card)
		if err != nil {
			return nil, nil, err
		}
		rel = filepath.ToSlash(rel)
		m, ps, err := LoadManifest(card)
		if err != nil {
			return nil, nil, err
		}
		for _, p := range ps {
			p.Path = rel + ":" + p.Path
			problems = append(problems, p)
		}
		if m == nil {
			continue
		}
		if first, dup := owner[m.Name]; dup {
			problems = append(problems, Problem{Code: "duplicate-name", Path: rel + ":/name", Severity: SeverityError,
				Message: fmt.Sprintf("name %s is already the name of %s", m.Name, first)})
			continue
		}
		owner[m.Name] = rel
		idx.Entries = append(idx.Entries, IndexEntry{Path: filepath.ToSlash(filepath.Dir(rel)), Name: m.Name, Version: m.Version, Type: m.Type, Tags: m.Tags})
	}
	return idx, problems, nil
}

// MarshalIndex renders an index as YAML, two-space indent.
func MarshalIndex(idx *Index) ([]byte, error) {
	if idx.Entries == nil {
		idx.Entries = []IndexEntry{}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(idx); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// SaveIndex writes an index as YAML to path, normally <root>/index.yaml.
func SaveIndex(path string, idx *Index) error {
	b, err := MarshalIndex(idx)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
