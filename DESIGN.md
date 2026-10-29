# Design direction

Direction chosen by the owner: **dark, poster-led**.

- **Theme:** dark only. It is a video player used mostly in the evening, and a dark frame keeps attention on the picture. No light theme is shipped.
- **Color:** the cover art carries the color. Each anime page takes its tint from AniList's `coverImage.color`. The app itself is a warm near-black with off-white text and **one accent, vermilion**, used only for the next thing to watch (the "continue" episode and the play action).
- **Palette:** background `#13110f`, surface `#1e1a17`, text `#f2ece4`, muted text `#a89f94`, accent `#ff5a36`. All text pairs pass WCAG AA (lowest is 4.86:1).
- **Typography:** titles and episode numbers in Barlow Condensed, a narrow face like poster and marquee lettering that also fits long romaji titles on small cards. Body text uses the system UI font, which renders natively on phones and needs no download.
- **Motif:** the episode number set large in the condensed face ("EP 7"), like a ticket stub, on the continue card and the player.
- **Dial:** ENERGY 2 / RHYTHM 2 / MOTION 1. Hover and focus states only, no entrance animations.
- **Name:** shown as a text wordmark only, no logo.
