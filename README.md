# SoraPlay

A self-hosted web app for watching anime with Indonesian subtitles, synced with your AniList account.

- Your AniList Watching list on the home page, with the next episode one tap away.
- A small AniList client around it: a weekly airing schedule in your own time zone, season browsing (any season, sorted by popularity, score or trend, filtered by format), and your full list by status with a title filter, one-tap +1 and an editor for status, progress, score, rewatches and dates. Anime pages show related seasons and AniList users' recommendations.
- Episodes stream with Indonesian subtitles burned in and play in the browser on phones, PCs and TVs. Regular anime comes from Source A (720p). Entries AniList marks as adult come from Source E; set `SOURCE_E_BASE=off` to turn that off.
- Finishing an episode (85% watched) updates your progress on AniList. Updates are queued, so a dropped connection or an AniList rate limit never loses one.
- The next two unwatched episodes of shows you watched in the last two weeks are saved on the server ahead of time, so they start instantly and play even when the source is down. Saved episodes are deleted once watched, and the Saved episodes page (in the account menu) shows what is on disk. Adult titles are never saved.
- Shows on your Watching list are checked for new episodes every 30 minutes; new ones appear on the home page and, with a Discord webhook set, as a message.
- Playback position is saved on the server, so an episode started on one device resumes on another. Intros and endings can be skipped using AniSkip timestamps.
- Each AniList entry is matched to the right show on the source automatically. When a match is wrong or missing, pick the show by hand and set an episode offset for sources that number split seasons continuously.
- No ads and no tracking. Source A video plays straight from the source's CDN. Source E's video host only serves the client that requested the stream, so those streams are relayed through the server (signed URLs, about 0.5 Mbps per viewer).

## Requirements

- Go 1.26 or later
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

The compose file runs the app as user `1000:1000` (change it to the owner of the folder holding `data/`) and publishes it on `127.0.0.1:8787` only. To reach it from your other devices over Tailscale with HTTPS (needed for installing it as an app on a phone), run on the server:

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
| `SOURCE_A_BASE` | `https://source-a.example` | Source for regular anime, for when its domain changes |
| `SOURCE_E_BASE` | `https://source-e.example` | Source for adult entries; `off` sends them to Source A |
| `DISCORD_WEBHOOK_URL` | (off) | Discord webhook that gets a message when a new episode of a show you are watching is up |
| `PREFETCH_DIR` | (off; `/saved` in Docker) | Folder for saving upcoming episodes ahead of time |
| `PREFETCH_MAX_GB` | `30` | Size cap for saved episodes |
| `PREFETCH_AHEAD` | `2` | Unwatched episodes kept saved per show |
| `SAVED_DIR` | `./saved` | Docker only: host folder mounted as `/saved` |
| `RELAY_ALL` | (off) | `1` relays every stream through the server, so video hosts see neither the viewer's address nor the app's `Origin` header (costs the server's upload bandwidth when watching away from home) |

## Access

The app has one AniList account per server and no login screen of its own: anyone who can open it can watch and update that account's progress. Keep it on a private network such as Tailscale, and do not publish it to the internet.

## Development

```bash
go test ./...                                    # unit tests
go test -tags live -run Live -v ./internal/...   # tests against the live AniList and source sites
go run ./cmd/spike search frieren                # try the source scraper from the terminal
```
