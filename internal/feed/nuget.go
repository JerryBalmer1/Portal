package feed

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// zipTime is every entry's modification time, so one card always packs to
// the same bytes.
var zipTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

const contentTypes = `<?xml version="1.0" encoding="utf-8"?>` +
	`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="nuspec" ContentType="application/octet" />` +
	`<Default Extension="psmdcp" ContentType="application/vnd.openxmlformats-package.core-properties+xml" />` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml" />` +
	`<Override PartName="/" ContentType="application/octet" />` +
	`</Types>`

type nuspec struct {
	XMLName  xml.Name `xml:"package"`
	XMLNS    string   `xml:"xmlns,attr"`
	Metadata struct {
		ID          string `xml:"id"`
		Version     string `xml:"version"`
		Authors     string `xml:"authors"`
		Description string `xml:"description"`
		Tags        string `xml:"tags,omitempty"`
	} `xml:"metadata"`
}

// Nupkg is a card as a .nupkg: <name>.nuspec (id the card's name, version
// its version, authors who pushed it, description its description, tags
// graphnode-card, its type and its tags), [Content_Types].xml, and every
// file of the card at its own path, manifest.yaml at the root, so a
// package extracted by nuget or Save-PSResource is the card's folder.
func Nupkg(c *Card, by string) ([]byte, error) {
	var spec nuspec
	spec.XMLNS = "http://schemas.microsoft.com/packaging/2013/05/nuspec.xsd"
	spec.Metadata.ID = c.Manifest.Name
	spec.Metadata.Version = c.Manifest.Version
	spec.Metadata.Authors = by
	spec.Metadata.Description = c.Manifest.Description
	spec.Metadata.Tags = strings.Join(append([]string{"graphnode-card", c.Manifest.Type}, c.Manifest.Tags...), " ")
	specXML, err := xml.MarshalIndent(spec, "", "  ")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	put := func(name string, data []byte) error {
		w, err := z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: zipTime})
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	if err := put(c.Manifest.Name+".nuspec", append([]byte(xml.Header), specXML...)); err != nil {
		return nil, err
	}
	if err := put("[Content_Types].xml", []byte(contentTypes)); err != nil {
		return nil, err
	}
	for _, f := range c.Files {
		if err := put(f.Path, f.Data); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// nugetResources reads the feed's service index (the feed URL,
// .../v3/index.json) and returns the publish URL (PackagePublish/2.0.0) and
// the flat container base (PackageBaseAddress/3.0.0).
func (cl Client) nugetResources(ctx context.Context, f Feed) (publish, flat string, err error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	res, err := cl.do(req)
	if err != nil {
		return "", "", err
	}
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", "", &UpstreamError{Status: res.StatusCode, Message: "service index " + f.URL}
	}
	var idx struct {
		Resources []struct {
			ID   string `json:"@id"`
			Type string `json:"@type"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(body, &idx); err != nil {
		return "", "", &UpstreamError{Status: res.StatusCode, Message: f.URL + " is not a NuGet v3 service index: " + err.Error()}
	}
	for _, r := range idx.Resources {
		switch {
		case strings.HasPrefix(r.Type, "PackagePublish/2.0.0") && publish == "":
			publish = r.ID
		case strings.HasPrefix(r.Type, "PackageBaseAddress/3.0.0") && flat == "":
			flat = r.ID
		}
	}
	if publish == "" || flat == "" {
		return "", "", &UpstreamError{Status: res.StatusCode, Message: f.URL + " names no PackagePublish/2.0.0 or PackageBaseAddress/3.0.0 resource"}
	}
	return publish, flat, nil
}

func (cl Client) nugetExists(ctx context.Context, f Feed, c *Card) (bool, error) {
	_, flat, err := cl.nugetResources(ctx, f)
	if err != nil {
		return false, err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(flat, "/")+"/"+strings.ToLower(c.Manifest.Name)+"/index.json", nil)
	res, err := cl.do(req)
	if err != nil {
		return false, err
	}
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if res.StatusCode != http.StatusOK {
		return false, &UpstreamError{Status: res.StatusCode, Message: "versions of " + c.Manifest.Name}
	}
	var v struct {
		Versions []string `json:"versions"`
	}
	_ = json.Unmarshal(body, &v)
	// The flat container lists normalised versions: lower case, no build metadata.
	want := strings.ToLower(strings.SplitN(c.Manifest.Version, "+", 2)[0])
	for _, have := range v.Versions {
		if strings.ToLower(have) == want {
			return true, nil
		}
	}
	return false, nil
}

// nugetPush PUTs the .nupkg to the publish URL as the multipart form NuGet's
// push protocol takes, with the feed's API key in X-NuGet-ApiKey when the
// feed names one. 409 from the feed is ErrExists. The digest is the
// .nupkg's SHA-256.
func (cl Client) nugetPush(ctx context.Context, f Feed, c *Card, by, key string) (string, error) {
	publish, _, err := cl.nugetResources(ctx, f)
	if err != nil {
		return "", err
	}
	pkg, err := Nupkg(c, by)
	if err != nil {
		return "", err
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("package", fmt.Sprintf("%s.%s.nupkg", c.Manifest.Name, c.Manifest.Version))
	if err != nil {
		return "", err
	}
	_, _ = part.Write(pkg)
	_ = mw.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, publish, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if key != "" {
		req.Header.Set("X-NuGet-ApiKey", key)
	}
	res, err := cl.do(req)
	if err != nil {
		return "", err
	}
	msg, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	res.Body.Close()
	switch {
	case res.StatusCode == http.StatusConflict:
		return "", fmt.Errorf("%w: %s %s is already in %s", ErrExists, c.Manifest.Name, c.Manifest.Version, f.ID)
	case res.StatusCode < 200 || res.StatusCode > 299:
		return "", &UpstreamError{Status: res.StatusCode, Message: "push " + c.Manifest.Name + " " + c.Manifest.Version + ": " + strings.TrimSpace(string(msg))}
	}
	return digest(pkg), nil
}
