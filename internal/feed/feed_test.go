package feed_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/JerryBalmer1/Portal/internal/feed"
	"github.com/JerryBalmer1/Portal/internal/feed/feedtest"
)

// cardDir is Tool.Git, copied from Portal.Tools (tools/Git at c330712).
const cardDir = "../../tests/fixtures/cards/Tool.Git"

func fixtureCard(t *testing.T) ([]byte, []feed.File) {
	t.Helper()
	m, err := os.ReadFile(filepath.Join(cardDir, "manifest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	inst, err := os.ReadFile(filepath.Join(cardDir, "install.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return m, []feed.File{{Path: "install.yaml", Data: inst}}
}

func TestNewCardValidatesWithGraphNode(t *testing.T) {
	m, files := fixtureCard(t)
	c, err := feed.NewCard(m, files)
	if err != nil {
		t.Fatal(err)
	}
	if c.Manifest.Name != "Tool.Git" || c.Manifest.Version != "0.1.0" || c.Manifest.Type != "tool" {
		t.Errorf("manifest %+v", c.Manifest)
	}
	if len(c.Files) != 2 || c.Files[0].Path != "manifest.yaml" {
		t.Errorf("files %v", c.Files)
	}
	if !strings.HasPrefix(c.Hash, "sha256:") || len(c.Hash) != 71 {
		t.Errorf("hash %q", c.Hash)
	}

	// Graph.Node's manifest schema refuses an unknown key and a bad version: code schema, path in manifest.yaml.
	_, err = feed.NewCard([]byte("name: Tool.Git\nversion: one\ntype: tool\ndescription: x\ncolour: red\n"), nil)
	var bad *feed.InvalidError
	if !errors.As(err, &bad) || !errors.Is(err, feed.ErrInvalid) {
		t.Fatalf("want InvalidError, got %v", err)
	}
	if bad.Name != "Tool.Git" || bad.Version != "one" {
		t.Errorf("name and version as far as decoded: %q %q", bad.Name, bad.Version)
	}
	paths := map[string]bool{}
	for _, p := range bad.Problems {
		if p.Code != "schema" {
			t.Errorf("code %q, want schema: %v", p.Code, p)
		}
		paths[p.Path] = true
	}
	if !paths["manifest.yaml:/version"] {
		t.Errorf("no problem at manifest.yaml:/version: %v", bad.Problems)
	}
	if _, err := feed.NewCard([]byte(": not yaml : ["), nil); !errors.Is(err, feed.ErrInvalid) {
		t.Errorf("not YAML: %v", err)
	}
}

func TestNewCardRefusesPathsItCannotPackage(t *testing.T) {
	m, _ := fixtureCard(t)
	for _, p := range []string{"", "/abs", "../up", "a/../b", `win\path`, "manifest.yaml", "Tool.Git.nuspec", "_rels/.rels", "[Content_Types].xml", "a//b", "a/.hidden/x"} {
		_, err := feed.NewCard(m, []feed.File{{Path: p, Data: []byte("x")}})
		var bad *feed.InvalidError
		if !errors.As(err, &bad) || bad.Problems[0].Code != "file" {
			t.Errorf("%q: want a file problem, got %v", p, err)
		}
	}
	if _, err := feed.NewCard(m, []feed.File{{Path: "a.txt"}, {Path: "A.TXT"}}); err == nil {
		t.Error("two paths differing only in case were accepted")
	}
	if _, err := feed.NewCard(m, []feed.File{{Path: "docs/read me.md", Data: []byte("x")}}); err != nil {
		t.Errorf("a nested path with a space: %v", err)
	}
}

func TestContentHashIgnoresOrderAndSeesEveryByte(t *testing.T) {
	a := []feed.File{{Path: "a", Data: []byte("1")}, {Path: "b", Data: []byte("2")}}
	b := []feed.File{a[1], a[0]}
	if feed.ContentHash(a) != feed.ContentHash(b) {
		t.Error("order changed the hash")
	}
	if feed.ContentHash(a) == feed.ContentHash([]feed.File{{Path: "a", Data: []byte("1")}, {Path: "b", Data: []byte("3")}}) {
		t.Error("a changed byte kept the hash")
	}
	if feed.ContentHash(a) == feed.ContentHash([]feed.File{{Path: "a", Data: []byte("1")}, {Path: "c", Data: []byte("2")}}) {
		t.Error("a renamed file kept the hash")
	}
}

func TestOCIRefAndManifest(t *testing.T) {
	m, files := fixtureCard(t)
	c, _ := feed.NewCard(m, files)
	base, repo, tag, err := feed.OCIRef(feed.Feed{URL: "http://127.0.0.1:8201/cards"}, c)
	if err != nil || base != "http://127.0.0.1:8201" || repo != "cards/tool.git" || tag != "0.1.0" {
		t.Errorf("ref %s %s %s %v", base, repo, tag, err)
	}
	c.Manifest.Version = "1.0.0+build.7"
	if _, _, tag, _ := feed.OCIRef(feed.Feed{URL: "http://r"}, c); tag != "1.0.0_build.7" {
		t.Errorf("build metadata tag %q", tag)
	}
	c.Manifest.Name = "Tool.Bad-"
	if _, _, _, err := feed.OCIRef(feed.Feed{URL: "http://r"}, c); !errors.Is(err, feed.ErrUnpackable) {
		t.Errorf("a name ending in - is not a repository component: %v", err)
	}
	c, _ = feed.NewCard(m, files)
	raw, blobs := feed.OCIManifest(c, "jerry", "2026-10-11T00:00:00.000Z")
	var man struct {
		MediaType, ArtifactType string
		Config                  struct{ MediaType, Digest string }
		Layers                  []struct {
			MediaType, Digest string
			Annotations       map[string]string
		}
		Annotations map[string]string
	}
	if err := json.Unmarshal(raw, &man); err != nil {
		t.Fatal(err)
	}
	if man.MediaType != feed.OCIManifestType || man.ArtifactType != feed.OCIArtifactType || man.Config.MediaType != "application/vnd.oci.empty.v1+json" {
		t.Errorf("types %+v", man)
	}
	var titles []string
	for _, l := range man.Layers {
		titles = append(titles, l.Annotations["org.opencontainers.image.title"])
		if _, ok := blobs[l.Digest]; !ok || l.MediaType != feed.OCIFileType {
			t.Errorf("layer %+v", l)
		}
	}
	if strings.Join(titles, ",") != "manifest.yaml,install.yaml" {
		t.Errorf("titles %v", titles)
	}
	for k, want := range map[string]string{"org.opencontainers.image.title": "Tool.Git", "org.opencontainers.image.version": "0.1.0",
		"dev.graphnode.card.type": "tool", "dev.graphnode.card.hash": c.Hash, "org.opencontainers.image.authors": "jerry"} {
		if man.Annotations[k] != want {
			t.Errorf("annotation %s = %q, want %q", k, man.Annotations[k], want)
		}
	}
}

func TestNupkgIsTheCardFolderWithANuspec(t *testing.T) {
	m, files := fixtureCard(t)
	c, _ := feed.NewCard(m, files)
	a, err := feed.Nupkg(c, "jerry")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := feed.Nupkg(c, "jerry")
	if !bytes.Equal(a, b) {
		t.Error("one card packed to two different .nupkg")
	}
	z, err := zip.NewReader(bytes.NewReader(a), int64(len(a)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range z.File {
		names = append(names, f.Name)
		if f.Name == "install.yaml" {
			rc, _ := f.Open()
			got, _ := io.ReadAll(rc)
			rc.Close()
			if !bytes.Equal(got, files[0].Data) {
				t.Error("install.yaml changed in the package")
			}
		}
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "Tool.Git.nuspec,[Content_Types].xml,install.yaml,manifest.yaml" {
		t.Errorf("entries %v", names)
	}
	id, v, err := feedtest.Nuspec(a)
	if err != nil || id != "Tool.Git" || v != "0.1.0" {
		t.Errorf("nuspec %s %s %v", id, v, err)
	}
}

func TestPushToARegistryThenAgainIsErrExists(t *testing.T) {
	reg := feedtest.NewRegistry(t)
	m, files := fixtureCard(t)
	c, _ := feed.NewCard(m, files)
	f := feed.Feed{ID: "oci", Kind: feed.KindOCI, URL: reg.URL + "/cards"}
	d, err := feed.Client{}.Push(context.Background(), f, c, "jerry", "2026-10-11T00:00:00.000Z")
	if err != nil {
		t.Fatal(err)
	}
	stored := reg.Manifest("cards/tool.git:0.1.0")
	if stored == nil || !strings.HasPrefix(d, "sha256:") {
		t.Fatalf("digest %q, stored %d bytes", d, len(stored))
	}
	if _, err := (feed.Client{}).Push(context.Background(), f, c, "jerry", "x"); !errors.Is(err, feed.ErrExists) {
		t.Errorf("second push: %v", err)
	}
}

func TestPushToNuGetWithItsKey(t *testing.T) {
	ng := feedtest.NewNuGet(t, "k3y")
	m, files := fixtureCard(t)
	c, _ := feed.NewCard(m, files)
	f := feed.Feed{ID: "nuget", Kind: feed.KindNuGet, URL: ng.Index(), TokenEnv: "PT_NUGET"}
	env := map[string]string{}
	cl := feed.Client{Getenv: func(k string) string { return env[k] }}
	if _, err := cl.Push(context.Background(), f, c, "jerry", "x"); !errors.Is(err, feed.ErrNoToken) {
		t.Errorf("no key: %v", err)
	}
	env["PT_NUGET"] = "wrong"
	var up *feed.UpstreamError
	if _, err := cl.Push(context.Background(), f, c, "jerry", "x"); !errors.As(err, &up) || up.Status != 401 {
		t.Errorf("wrong key: %v", err)
	}
	env["PT_NUGET"] = "k3y"
	d, err := cl.Push(context.Background(), f, c, "jerry", "x")
	if err != nil {
		t.Fatal(err)
	}
	pkg := ng.Package("Tool.Git", "0.1.0")
	if pkg == nil {
		t.Fatal("not stored")
	}
	want, _ := feed.Nupkg(c, "jerry")
	if !bytes.Equal(pkg, want) || d != digestOf(want) {
		t.Errorf("digest %s", d)
	}
	if _, err := cl.Push(context.Background(), f, c, "jerry", "x"); !errors.Is(err, feed.ErrExists) {
		t.Errorf("second push: %v", err)
	}
}

func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}
