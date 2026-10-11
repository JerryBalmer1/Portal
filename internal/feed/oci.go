package feed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Media types of a card pushed as an OCI artifact. The config is the OCI
// empty descriptor, as ORAS writes for artifacts with no config, so
// `oras pull <registry>/<repo>:<tag>` restores the card's files by their
// org.opencontainers.image.title annotations (docs/design.md 40).
const (
	OCIManifestType = "application/vnd.oci.image.manifest.v1+json"
	OCIArtifactType = "application/vnd.graphnode.card.v1"
	OCIFileType     = "application/vnd.graphnode.card.file.v1"
	ociEmptyType    = "application/vnd.oci.empty.v1+json"
)

var ociEmpty = []byte("{}")

// ociComponent is one component of an OCI repository name (distribution spec).
var (
	ociComponent = regexp.MustCompile(`^[a-z0-9]+((\.|_|__|-+)[a-z0-9]+)*$`)
	ociTag       = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$`)
)

type ociDescriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int               `json:"size"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type ociManifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	MediaType     string            `json:"mediaType"`
	ArtifactType  string            `json:"artifactType"`
	Config        ociDescriptor     `json:"config"`
	Layers        []ociDescriptor   `json:"layers"`
	Annotations   map[string]string `json:"annotations"`
}

// OCIRef is where a card goes in an OCI feed: the registry's base URL
// (scheme and host), the repository (the feed URL's path, then the card's
// name in lower case: http://127.0.0.1:8201/cards and Tool.Git give
// cards/tool.git) and the tag (the version, with SemVer's + written _,
// since a tag may not hold +).
func OCIRef(f Feed, c *Card) (base, repo, tag string, err error) {
	u, err := url.Parse(f.URL)
	if err != nil {
		return "", "", "", err
	}
	base = u.Scheme + "://" + u.Host
	var parts []string
	for _, s := range strings.Split(strings.Trim(u.Path, "/"), "/") {
		if s != "" {
			parts = append(parts, s)
		}
	}
	parts = append(parts, strings.ToLower(c.Manifest.Name))
	for _, s := range parts {
		if !ociComponent.MatchString(s) {
			return "", "", "", fmt.Errorf("%w: %q is not an OCI repository name component (lower-case letters and digits, joined by . _ or -)", ErrUnpackable, s)
		}
	}
	tag = strings.ReplaceAll(c.Manifest.Version, "+", "_")
	if !ociTag.MatchString(tag) {
		return "", "", "", fmt.Errorf("%w: version %q is not an OCI tag", ErrUnpackable, c.Manifest.Version)
	}
	return base, strings.Join(parts, "/"), tag, nil
}

// OCIManifest is the artifact manifest a card is pushed as, and its blobs by
// digest. Every file is one layer; the manifest's annotations carry the
// card's name, version, type, description, content hash and who pushed it.
func OCIManifest(c *Card, by, created string) ([]byte, map[string][]byte) {
	blobs := map[string][]byte{digest(ociEmpty): ociEmpty}
	m := ociManifest{
		SchemaVersion: 2,
		MediaType:     OCIManifestType,
		ArtifactType:  OCIArtifactType,
		Config:        ociDescriptor{MediaType: ociEmptyType, Digest: digest(ociEmpty), Size: len(ociEmpty)},
		Annotations: map[string]string{
			"org.opencontainers.image.title":       c.Manifest.Name,
			"org.opencontainers.image.version":     c.Manifest.Version,
			"org.opencontainers.image.description": c.Manifest.Description,
			"org.opencontainers.image.created":     created,
			"org.opencontainers.image.authors":     by,
			"dev.graphnode.card.type":              c.Manifest.Type,
			"dev.graphnode.card.hash":              c.Hash,
		},
	}
	for _, f := range c.Files {
		d := digest(f.Data)
		blobs[d] = f.Data
		m.Layers = append(m.Layers, ociDescriptor{MediaType: OCIFileType, Digest: d, Size: len(f.Data),
			Annotations: map[string]string{"org.opencontainers.image.title": f.Path}})
	}
	b, _ := json.Marshal(m)
	return b, blobs
}

func (cl Client) ociExists(ctx context.Context, f Feed, c *Card) (bool, error) {
	base, repo, tag, err := OCIRef(f, c)
	if err != nil {
		return false, err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, base+"/v2/"+repo+"/manifests/"+tag, nil)
	req.Header.Set("Accept", OCIManifestType)
	res, err := cl.do(req)
	if err != nil {
		return false, err
	}
	res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	}
	return false, &UpstreamError{Status: res.StatusCode, Message: "HEAD manifest " + repo + ":" + tag}
}

// ociPush uploads every blob the registry lacks (monolithic upload: POST,
// then PUT with the digest), then PUTs the manifest under the tag. The
// digest is the manifest's, as the registry names it.
func (cl Client) ociPush(ctx context.Context, f Feed, c *Card, by, created string) (string, error) {
	base, repo, tag, err := OCIRef(f, c)
	if err != nil {
		return "", err
	}
	manifest, blobs := OCIManifest(c, by, created)
	for d, data := range blobs {
		req, _ := http.NewRequestWithContext(ctx, http.MethodHead, base+"/v2/"+repo+"/blobs/"+d, nil)
		res, err := cl.do(req)
		if err != nil {
			return "", err
		}
		res.Body.Close()
		if res.StatusCode == http.StatusOK {
			continue
		}
		req, _ = http.NewRequestWithContext(ctx, http.MethodPost, base+"/v2/"+repo+"/blobs/uploads/", nil)
		res, err = cl.do(req)
		if err != nil {
			return "", err
		}
		res.Body.Close()
		if res.StatusCode != http.StatusAccepted {
			return "", &UpstreamError{Status: res.StatusCode, Message: "start blob upload to " + repo}
		}
		loc, err := res.Request.URL.Parse(res.Header.Get("Location"))
		if err != nil || res.Header.Get("Location") == "" {
			return "", &UpstreamError{Status: res.StatusCode, Message: "blob upload to " + repo + " gave no Location"}
		}
		q := loc.Query()
		q.Set("digest", d)
		loc.RawQuery = q.Encode()
		req, _ = http.NewRequestWithContext(ctx, http.MethodPut, loc.String(), bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/octet-stream")
		req.ContentLength = int64(len(data))
		res, err = cl.do(req)
		if err != nil {
			return "", err
		}
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		res.Body.Close()
		if res.StatusCode != http.StatusCreated {
			return "", &UpstreamError{Status: res.StatusCode, Message: "upload blob " + d + ": " + strings.TrimSpace(string(msg))}
		}
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, base+"/v2/"+repo+"/manifests/"+tag, bytes.NewReader(manifest))
	req.Header.Set("Content-Type", OCIManifestType)
	res, err := cl.do(req)
	if err != nil {
		return "", err
	}
	msg, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return "", &UpstreamError{Status: res.StatusCode, Message: "put manifest " + repo + ":" + tag + ": " + strings.TrimSpace(string(msg))}
	}
	if d := res.Header.Get("Docker-Content-Digest"); d != "" {
		return d, nil
	}
	return digest(manifest), nil
}
