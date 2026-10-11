package feed

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
)

// Feed kinds.
const (
	KindOCI   = "oci"
	KindNuGet = "nuget"
)

// Feed is one entry of the merged config's feeds (portal.base.yaml, then the
// tenant's overlay). ID is its name; URL is the registry (and repository
// namespace) for oci, the service index for nuget. TokenEnv names the
// environment variable holding the feed's API key, read when a push needs
// it and never stored or served.
type Feed struct {
	ID         string
	Label      string
	Kind       string
	URL        string
	Visibility string
	TokenEnv   string
}

// Errors of a push, mapped by the server onto 409, 422 and 502.
var (
	ErrExists     = errors.New("already in the feed")
	ErrNoToken    = errors.New("feed key not set")
	ErrUnpackable = errors.New("cannot be packaged for this feed")
)

// UpstreamError is a feed that failed or refused a call.
type UpstreamError struct {
	Status  int
	Message string
}

func (e *UpstreamError) Error() string {
	if e.Status == 0 {
		return e.Message
	}
	return fmt.Sprintf("%d: %s", e.Status, e.Message)
}

// Client pushes cards. HTTP and Getenv are injectable for tests; nil means
// http.DefaultClient and os.Getenv.
type Client struct {
	HTTP   *http.Client
	Getenv func(string) string
}

func (cl Client) do(req *http.Request) (*http.Response, error) {
	h := cl.HTTP
	if h == nil {
		h = http.DefaultClient
	}
	res, err := h.Do(req)
	if err != nil {
		return nil, &UpstreamError{Message: err.Error()}
	}
	return res, nil
}

// Key is the feed's API key: "" when the feed names no variable, ErrNoToken
// when it names one that is not set.
func (cl Client) Key(f Feed) (string, error) {
	if f.TokenEnv == "" {
		return "", nil
	}
	getenv := cl.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	if k := getenv(f.TokenEnv); k != "" {
		return k, nil
	}
	return "", fmt.Errorf("%w: feed %s reads its key from %s, which is not set", ErrNoToken, f.ID, f.TokenEnv)
}

// Push sends the card to the feed, by (the pusher's name) as its author and
// created as its time, and returns the feed's digest of what it stored: the
// OCI manifest's digest, or the .nupkg's SHA-256. A card the feed already
// holds at that version is ErrExists, and nothing is sent.
func (cl Client) Push(ctx context.Context, f Feed, c *Card, by, created string) (string, error) {
	key, err := cl.Key(f)
	if err != nil {
		return "", err
	}
	if f.Kind == KindOCI {
		if _, _, _, err := OCIRef(f, c); err != nil {
			return "", err
		}
	}
	var exists bool
	switch f.Kind {
	case KindOCI:
		exists, err = cl.ociExists(ctx, f, c)
	case KindNuGet:
		exists, err = cl.nugetExists(ctx, f, c)
	default:
		return "", fmt.Errorf("feed %s has kind %q", f.ID, f.Kind)
	}
	if err != nil {
		return "", err
	}
	if exists {
		return "", fmt.Errorf("%w: %s %s is already in %s", ErrExists, c.Manifest.Name, c.Manifest.Version, f.ID)
	}
	if f.Kind == KindOCI {
		return cl.ociPush(ctx, f, c, by, created)
	}
	return cl.nugetPush(ctx, f, c, by, key)
}
