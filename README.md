# SoraPlay

A self-hosted web app for watching anime with Indonesian subtitles, synced with your AniList account.

- Your AniList Watching list on the home page, with the next episode one tap away.
- Episodes stream from Source A (720p, Indonesian subtitles burned in) and play in the browser on phones, PCs and TVs.
- Finishing an episode (85% watched) updates your progress on AniList. Updates are queued, so a dropped connection or an AniList rate limit never loses one.
- Each AniList entry is matched to the right show on the source automatically. When a match is wrong or missing, pick the show by hand and set an episode offset for sources that number split seasons continuously.
- No ads and no tracking. The video plays straight from the source's CDN; the server only finds the stream link.

## Requirements

- Go 1.25 or later
- Node.js 22 or later (to build the web frontend)
- An AniList account

## AniList API client

1. Open <https://anilist.co/settings/developer> and choose **Create New Client**.
2. Set **Redirect URL** to `<your app URL>/auth/callback`, for example `http://localhost:8787/auth/callback` when running locally, or `https://homelab.example.ts.net/auth/callback` on a server. A client has one redirect URL, so create a separate client for each address you use.
3. Copy the client ID and secret into `.env` (see `.env.example`), and set `NONTON_PUBLIC_URL` to the same app URL.

Without a client the app still works for searching and watching, but cannot log in or sync progress.

## Running locally

```bash
cp .env.example .env    # then fill in the AniList client
cd web && npm ci && npm run build && cd ..
go run ./cmd/nonton
```

Open <http://localhost:8787>. State (login, source matches, pending progress) is stored in `data/nonton.db`.

For frontend development, run `npm run dev` in `web/` while the Go server is running; Vite serves the app on port 5180 and forwards API calls to port 8787.

## Running with Docker

```bash
cp .env.example .env    # fill in the client and NONTON_PUBLIC_URL
docker compose up -d --build
```

The compose file publishes the app on `127.0.0.1:8787` only. To reach it from your other devices over Tailscale with HTTPS (needed for installing it as an app on a phone), run on the server:

```bash
tailscale serve --bg 8787
```

The app is then at `https://<machine>.<tailnet>.ts.net`; use that as `NONTON_PUBLIC_URL` and in the AniList client's redirect URL.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `NONTON_PUBLIC_URL` | `http://localhost:8787` | Address the browser uses; the login callback is built from it |
| `ANILIST_CLIENT_ID` | | AniList API client ID |
| `ANILIST_CLIENT_SECRET` | | AniList API client secret |
| `NONTON_ADDR` | `:8787` | Listen address |
| `NONTON_DATA` | `data` | Folder for the SQLite database |
| `SOURCE_A_BASE` | `https://source-a.example` | Source site address, for when its domain changes |

## Access

The app has one AniList account per server and no login screen of its own: anyone who can open it can watch and update that account's progress. Keep it on a private network such as Tailscale, and do not publish it to the internet.

## Development

```bash
go test ./...                                    # unit tests
go test -tags live -run Live -v ./internal/...   # tests against the live AniList and source sites
go run ./cmd/spike search frieren                # try the source scraper from the terminal
```
