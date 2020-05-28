// Package sponsors provides GitHub sponsors management.
package sponsors

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shurcooL/githubv4"
)

// pixel is a png used for missing avatars.
var pixel []byte

// initialize gray pixel for missing avatar responses.
func init() {
	var buf bytes.Buffer
	r := image.Rect(0, 0, 1, 1)
	img := image.NewRGBA(r)
	c := color.RGBA{0xF8, 0xF9, 0xFA, 0xFF}
	draw.Draw(img, r, &image.Uniform{c}, image.ZP, draw.Src)
	png.Encode(&buf, img)
	pixel = buf.Bytes()
}

// Sponsor model.
type Sponsor struct {
	// Name of the sponsor.
	Name string

	// Login name of the sponsor.
	Login string

	// AvatarURL of the sponsor.
	AvatarURL string
}

// Server manager.
type Server struct {
	// URL is the url of the server.
	URL string

	// Client is the github client.
	Client *githubv4.Client

	// CacheTTL is the duration until the cache expires.
	CacheTTL time.Duration

	// cache
	mu             sync.Mutex
	cacheTimestamp time.Time
	cache          []Sponsor
}

// ServeHTTP implementation.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	path := r.URL.Path

	// logging
	start := time.Now()
	log.Printf("%s %s", r.Method, path)
	defer func() {
		log.Printf("%s %s -> %s", r.Method, path, time.Since(start))
	}()

	// prime cache
	err := s.primeCache(ctx)
	if err != nil {
		log.Printf("error priming cache: %s", err)
		http.Error(w, "Error fetching sponsors", http.StatusInternalServerError)
		return
	}

	// routing
	switch {
	case strings.HasPrefix(path, "/sponsor/markdown"):
		s.serveMarkdown(w, r)
	case strings.HasPrefix(path, "/sponsor/avatar"):
		s.serveAvatar(w, r)
	case strings.HasPrefix(path, "/sponsor/profile"):
		s.serveProfile(w, r)
	default:
		http.Error(w, "Not Found", http.StatusNotImplemented)
	}
}

// serveMarkdown serves a list of markdown links which you can copy/paste into your Readme.
func (s *Server) serveMarkdown(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown")
	for i := 0; i < 100; i++ {
		fmt.Fprintf(w, `[<img src="%s/sponsor/avatar/%d" width="35">](%s/sponsor/profile/%d)`, s.URL, i, s.URL, i)
		fmt.Fprintf(w, "\n")
	}
}

// serveAvatar redirects to a sponsor's avatar image.
func (s *Server) serveAvatar(w http.ResponseWriter, r *http.Request) {
	// /sponsor/avatar/{index}
	index := strings.Replace(r.URL.Path, "/sponsor/avatar/", "", 1)
	n, err := strconv.Atoi(index)
	if err != nil {
		log.Printf("error parsing index: %s", err)
		http.Error(w, "Sponsor index must be a number", http.StatusBadRequest)
		return
	}

	// check index bounds
	if n > len(s.cache)-1 {
		w.Header().Set("Content-Type", "image/png")
		io.Copy(w, bytes.NewReader(pixel))
		return
	}

	// redirect to avatar
	sponsor := s.cache[n]
	w.Header().Set("Location", sponsor.AvatarURL)
	w.WriteHeader(http.StatusTemporaryRedirect)
	fmt.Fprintf(w, "Redirecting to %s", sponsor.AvatarURL)
}

// serveProfile redirects to a sponsor's profile.
func (s *Server) serveProfile(w http.ResponseWriter, r *http.Request) {
	// /sponsor/profile/{index}
	index := strings.Replace(r.URL.Path, "/sponsor/profile/", "", 1)
	n, err := strconv.Atoi(index)
	if err != nil {
		log.Printf("error parsing index: %s", err)
		http.Error(w, "Sponsor index must be a number", http.StatusBadRequest)
		return
	}

	// check index bounds
	if n > len(s.cache)-1 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// redirect to profile
	sponsor := s.cache[n]
	url := fmt.Sprintf("https://github.com/%s", sponsor.Login)
	w.Header().Set("Location", url)
	w.WriteHeader(http.StatusTemporaryRedirect)
	fmt.Fprintf(w, "Redirecting to %s", url)
}

// primeCache implementation.
func (s *Server) primeCache(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// check ttl
	if time.Since(s.cacheTimestamp) <= s.CacheTTL {
		return nil
	}

	// fetch
	log.Printf("cache miss, fetching sponsors")
	sponsors, err := s.getSponsors(ctx)
	if err != nil {
		return err
	}

	s.cache = sponsors
	s.cacheTimestamp = time.Now()
	return nil
}

// getSponsors implementation.
func (s *Server) getSponsors(ctx context.Context) ([]Sponsor, error) {
	var sponsors []Sponsor
	var q sponsorships
	var cursor string

	for {
		err := s.Client.Query(ctx, &q, map[string]interface{}{
			"cursor": githubv4.String(cursor),
		})

		if err != nil {
			return nil, err
		}

		for _, edge := range q.Viewer.SponsorshipsAsMaintainer.Edges {
			sponsor := edge.Node.Sponsor
			sponsors = append(sponsors, sponsor)
		}

		if !q.Viewer.SponsorshipsAsMaintainer.PageInfo.HasNextPage {
			break
		}

		cursor = q.Viewer.SponsorshipsAsMaintainer.PageInfo.EndCursor
	}

	return sponsors, nil
}

// sponsorships query.
type sponsorships struct {
	Viewer struct {
		Login                    string
		SponsorshipsAsMaintainer struct {
			PageInfo struct {
				EndCursor   string
				HasNextPage bool
			}

			Edges []struct {
				Node struct {
					Sponsor Sponsor
				}
				Cursor string
			}
		} `graphql:"sponsorshipsAsMaintainer(first: 100, after: $cursor)"`
	}
}
