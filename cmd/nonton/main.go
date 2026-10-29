// Command nonton is the web server: AniList-synced anime streaming with
// Indonesian subtitles.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/vermilion10/nonton/internal/anilist"
	"github.com/vermilion10/nonton/internal/aniskip"
	"github.com/vermilion10/nonton/internal/config"
	"github.com/vermilion10/nonton/internal/httpc"
	"github.com/vermilion10/nonton/internal/progress"
	"github.com/vermilion10/nonton/internal/provider"
	"github.com/vermilion10/nonton/internal/provider/sourcee"
	"github.com/vermilion10/nonton/internal/provider/sourcea"
	"github.com/vermilion10/nonton/internal/server"
	"github.com/vermilion10/nonton/internal/store"
	"github.com/vermilion10/nonton/web"
)

func main() {
	cfg := config.Load()
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(filepath.Join(cfg.DataDir, "nonton.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	al := anilist.New()
	var adult provider.Provider
	if cfg.SourceEBase != "off" {
		adult = sourcee.New(cfg.SourceEBase, httpc.New())
	}
	worker := progress.NewWorker(st, al)
	srv := &server.Server{
		Cfg:           cfg,
		Store:         st,
		AniList:       al,
		Provider:      sourcea.New(cfg.SourceABase, httpc.New()),
		AdultProvider: adult,
		Progress:      worker,
		AniSkip:       aniskip.New(),
		Static:        web.Dist(),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go worker.Run(ctx)

	hs := &http.Server{Addr: cfg.Addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go srv.Track(ctx)
	go srv.Save(ctx)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hs.Shutdown(shutdown)
	}()
	if cfg.ClientID == "" {
		log.Print("ANILIST_CLIENT_ID is not set: AniList login is disabled")
	}
	log.Printf("listening on %s (public URL %s)", cfg.Addr, cfg.PublicURL)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
