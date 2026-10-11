package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/JerryBalmer1/Portal/internal/feed"
	"github.com/JerryBalmer1/Portal/internal/gen/api"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

// pushTimeout bounds one push, every call to the feed included.
const pushTimeout = 2 * time.Minute

// ListFeeds is the tenant's feeds and the cards each holds, as a graph
// (docs/design.md 40).
func (s *Server) ListFeeds(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if s.tenantOK(w, name) {
		writeJSON(w, http.StatusOK, s.t.FeedsEnvelope(time.Now().UTC().Format(store.StampLayout)))
	}
}

// PushCard publishes one card to a feed. The middleware has already refused
// anonymous (401) and a reader (403); every push that names a feed the
// tenant has is one ledger line, whatever comes of it (tenant.Push).
func (s *Server) PushCard(w http.ResponseWriter, r *http.Request, name api.Tenant, feedID string) {
	if !s.tenantOK(w, name) {
		return
	}
	if _, err := s.t.Feed(feedID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", err.Error(), nil)
		return
	}
	manifest, files, err := readCard(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "a push is multipart/form-data: one part manifest (the card's manifest.yaml) and one part file per other file, its filename the path in the card's folder: "+err.Error(), nil)
		return
	}
	// Pushes are one at a time per tenant, so a feed that hangs must not hold the rest for long.
	ctx, cancel := context.WithTimeout(r.Context(), pushTimeout)
	defer cancel()
	e, err := s.t.Push(ctx, s.feeds, principal(r), feedID, manifest, files)
	var bad *feed.InvalidError
	var up *feed.UpstreamError
	switch {
	case err == nil:
		writeJSON(w, http.StatusCreated, e)
	case errors.As(err, &bad):
		writeError(w, http.StatusUnprocessableEntity, "invalid", fmt.Sprintf("feed %s refuses the card", feedID), problems(bad.Problems))
	case errors.Is(err, feed.ErrUnpackable):
		writeError(w, http.StatusUnprocessableEntity, "invalid", err.Error(), nil)
	case errors.Is(err, tenant.ErrDuplicate):
		writeError(w, http.StatusConflict, "conflict", err.Error(), nil)
	case errors.Is(err, feed.ErrNoToken):
		writeError(w, http.StatusUnprocessableEntity, "no_token", err.Error(), nil)
	case errors.As(err, &up):
		writeError(w, http.StatusBadGateway, "upstream", fmt.Sprintf("feed %s: %s", feedID, up.Error()), nil)
	default:
		writeError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
	}
}

// readCard reads the push body: exactly one part named manifest, and any
// number named file, each file's path its filename as sent. Go's
// Part.FileName keeps only the base name, so the path is read from the
// Content-Disposition header itself.
func readCard(w http.ResponseWriter, r *http.Request) ([]byte, []feed.File, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, nil, err
	}
	var manifest []byte
	var files []feed.File
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		data, err := io.ReadAll(part)
		if err != nil {
			return nil, nil, err
		}
		_, params, _ := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		switch params["name"] {
		case "manifest":
			if manifest != nil {
				return nil, nil, errors.New("two manifest parts")
			}
			manifest = data
		case "file":
			files = append(files, feed.File{Path: params["filename"], Data: data})
		default:
			return nil, nil, fmt.Errorf("unknown part %q", params["name"])
		}
	}
	if manifest == nil {
		return nil, nil, errors.New("no manifest part")
	}
	return manifest, files, nil
}
