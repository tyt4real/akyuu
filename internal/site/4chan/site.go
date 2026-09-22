// Package fourchan provides the 4chan.org site module.
// Platform: fourchan (HTML-only, no JSON API)
package fourchan

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"akyuu/internal/adapter"
	"akyuu/internal/adapter/fourchan"
	"akyuu/internal/site"
	"akyuu/internal/siteregistry"
)

func init() {
	siteregistry.Register(
		site.NewSiteConfig().
			WithName("4chan").
			WithBaseURL("https://boards.4chan.org").
			WithPlatform(adapter.PlatformFourChan).
			WithAPIAvailable(false).
			WithPollInterval(60 * time.Second).
			WithRateLimitPerSec(1).
			WithMaxConcurrentReqs(1).
			WithBoards([]site.Board{
				// SFW boards
				{Code: "g", Title: "Technology", NSFW: false, PollInterval: 30 * time.Second},
				{Code: "a", Title: "Anime & Manga", NSFW: false},
				{Code: "adv", Title: "Advice", NSFW: false},
				{Code: "an", Title: "Animals & Nature", NSFW: false},
				{Code: "biz", Title: "Business & Finance", NSFW: false},
				{Code: "c", Title: "Anime/Cute", NSFW: false},
				{Code: "cgl", Title: "Cosplay & EGL", NSFW: false},
				{Code: "ck", Title: "Food & Cooking", NSFW: false},
				{Code: "cm", Title: "Cute/Male", NSFW: false},
				{Code: "co", Title: "Comics & Cartoons", NSFW: false},
				{Code: "diy", Title: "Do-It-Yourself", NSFW: false, DownloadFull: boolPtr(true)},
				{Code: "fa", Title: "Fashion", NSFW: false},
				{Code: "fit", Title: "Fitness", NSFW: false},
				{Code: "gd", Title: "Graphic Design", NSFW: false},
				{Code: "his", Title: "History & Humanities", NSFW: false},
				{Code: "int", Title: "International", NSFW: false},
				{Code: "jp", Title: "Otaku Culture", NSFW: false},
				{Code: "k", Title: "Weapons", NSFW: false},
				{Code: "lgbt", Title: "LGBT", NSFW: false},
				{Code: "lit", Title: "Literature", NSFW: false},
				{Code: "m", Title: "Mecha", NSFW: false},
				{Code: "mlp", Title: "Pony", NSFW: false},
				{Code: "mu", Title: "Music", NSFW: false},
				{Code: "n", Title: "Transportation", NSFW: false},
				{Code: "news", Title: "Current News", NSFW: false},
				{Code: "o", Title: "Auto", NSFW: false},
				{Code: "out", Title: "Outdoors", NSFW: false},
				{Code: "p", Title: "Photography", NSFW: false},
				{Code: "po", Title: "Papercraft & Origami", NSFW: false},
				{Code: "pw", Title: "Professional Wrestling", NSFW: false},
				{Code: "qst", Title: "Quests", NSFW: false},
				{Code: "sci", Title: "Science & Math", NSFW: false},
				{Code: "sp", Title: "Sports", NSFW: false},
				{Code: "tg", Title: "Traditional Games", NSFW: false},
				{Code: "toy", Title: "Toys", NSFW: false},
				{Code: "trv", Title: "Travel", NSFW: false},
				{Code: "tv", Title: "Television & Film", NSFW: false},
				{Code: "v", Title: "Video Games", NSFW: false},
				{Code: "vg", Title: "Video Game Generals", NSFW: false},
				{Code: "vip", Title: "Very Important Posts", NSFW: false},
				{Code: "vm", Title: "Video Games/Multiplayer", NSFW: false},
				{Code: "vmg", Title: "Video Games/Mobile", NSFW: false},
				{Code: "vp", Title: "Pokémon", NSFW: false},
				{Code: "vr", Title: "Retro Games", NSFW: false},
				{Code: "vrpg", Title: "Video Games/RPG", NSFW: false},
				{Code: "vst", Title: "Video Games/Strategy", NSFW: false},
				{Code: "vt", Title: "Virtual YouTubers", NSFW: false},
				{Code: "w", Title: "Anime/Wallpapers", NSFW: false},
				{Code: "wsg", Title: "Worksafe GIF", NSFW: false},
				{Code: "wsr", Title: "Worksafe Requests", NSFW: false},
				{Code: "x", Title: "Paranormal", NSFW: false},
				{Code: "xs", Title: "Extreme Sports", NSFW: false},

				// NSFW boards
				{Code: "b", Title: "Random", NSFW: true},
				{Code: "bant", Title: "International/Random", NSFW: true},
				{Code: "aco", Title: "Adult Cartoons", NSFW: true},
				{Code: "d", Title: "Hentai/Alternative", NSFW: true},
				{Code: "e", Title: "Ecchi", NSFW: true},
				{Code: "f", Title: "Flash", NSFW: true},
				{Code: "gif", Title: "Adult GIF", NSFW: true},
				{Code: "h", Title: "Hentai", NSFW: true},
				{Code: "hc", Title: "Hardcore", NSFW: true},
				{Code: "hm", Title: "Handsome Men", NSFW: true},
				{Code: "hr", Title: "High Resolution", NSFW: true},
				{Code: "i", Title: "Oekaki", NSFW: true},
				{Code: "ic", Title: "Artwork/Critique", NSFW: true},
				{Code: "pol", Title: "Politically Incorrect", NSFW: true},
				{Code: "r", Title: "Adult Requests", NSFW: true},
				{Code: "r9k", Title: "ROBOT9001", NSFW: true},
				{Code: "s", Title: "Sexy Beautiful Women", NSFW: true},
				{Code: "s4s", Title: "Shit 4chan Says", NSFW: true},
				{Code: "soc", Title: "Cams & Meetups", NSFW: true},
				{Code: "t", Title: "Torrents", NSFW: true},
				{Code: "trash", Title: "Off-Topic", NSFW: true},
				{Code: "u", Title: "Yuri", NSFW: true},
				{Code: "wg", Title: "Wallpapers/General", NSFW: true},
				{Code: "y", Title: "Yaoi", NSFW: true},
			}),
	)
}

// NewAdapter creates a fourchan adapter for 4chan.
func NewAdapter(ctx context.Context, baseURL, userAgent string, transport http.RoundTripper, logger *slog.Logger) (adapter.Adapter, error) {
	return fourchan.New(baseURL, userAgent, transport, logger)
}

func boolPtr(b bool) *bool { return &b }
