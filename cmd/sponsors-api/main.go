package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/shurcooL/githubv4"
	"github.com/tj/go/env"
	"golang.org/x/oauth2"

	sponsors "github.com/tj/sponsors-api"
)

func main() {
	cacheTTL := flag.String("cache-ttl", "1h", "Sponsor cache duration")
	flag.Parse()

	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)
	httpClient := oauth2.NewClient(context.Background(), src)
	client := githubv4.NewClient(httpClient)

	ttl, err := time.ParseDuration(*cacheTTL)
	if err != nil {
		log.Fatalf("error parsing cache ttl: %s", err)
	}

	s := &sponsors.Server{
		URL:      env.GetDefault("URL", "http://localhost:3000"),
		CacheTTL: ttl,
		Client:   client,
	}

	addr := "0.0.0.0:" + env.GetDefault("PORT", "3000")
	log.Printf("Listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, s))
}
