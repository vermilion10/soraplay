// Package config reads settings from the environment, after loading an
// optional .env file in the working directory.
package config

import (
	"bufio"
	"os"
	"strings"
)

type Config struct {
	Addr          string // listen address
	DataDir       string // SQLite database and other state
	PublicURL     string // URL the browser uses to reach the app; used for the OAuth redirect
	ClientID      string // AniList API client
	ClientSecret  string
	SourceABase string
	SourceEBase   string // source for adult entries; "off" disables it
	// RelayAll sends every stream through the server, so video hosts never
	// see the viewer's address or this app's Origin.
	RelayAll bool
}

func Load() Config {
	loadDotEnv(".env")
	return Config{
		Addr:          env("NONTON_ADDR", ":8787"),
		DataDir:       env("NONTON_DATA", "data"),
		PublicURL:     strings.TrimRight(env("NONTON_PUBLIC_URL", "http://localhost:8787"), "/"),
		ClientID:      env("ANILIST_CLIENT_ID", ""),
		ClientSecret:  env("ANILIST_CLIENT_SECRET", ""),
		SourceABase: env("SOURCE_A_BASE", "https://source-a.example"),
		SourceEBase:   env("SOURCE_E_BASE", "https://source-e.example"),
		RelayAll:      env("RELAY_ALL", "") == "1",
	}
}

// RedirectURI is the OAuth callback registered with the AniList client.
func (c Config) RedirectURI() string { return c.PublicURL + "/auth/callback" }

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

// loadDotEnv sets KEY=VALUE lines from path without overriding variables
// that are already set.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
}
