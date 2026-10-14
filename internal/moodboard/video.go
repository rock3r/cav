package moodboard

import (
	"net/url"
	"regexp"
	"strings"
)

// Web video is only ever embedded, never downloaded. Everything the page embeds is built
// from an id that ParseVideoURL took out of a link and checked. The link in the file is never
// used as an embed address, so a hand-edited file cannot make the page load another site.

const (
	YouTube = "youtube"
	Vimeo   = "vimeo"
)

var (
	youtubeID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	vimeoID   = regexp.MustCompile(`^[0-9]{1,12}$`)
	vimeoHash = regexp.MustCompile(`^[A-Za-z0-9]{6,32}$`)
)

// ParseVideoURL reads a YouTube or Vimeo link. It accepts only http and https links without
// a user name, on the hosts below, and ids made of safe characters.
func ParseVideoURL(raw string) (provider, id, hash string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Opaque != "" {
		return "", "", "", false
	}
	if p := u.Port(); p != "" && p != "443" && p != "80" {
		return "", "", "", false
	}
	host := strings.ToLower(u.Hostname())
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch host {
	case "youtube.com", "www.youtube.com", "m.youtube.com", "youtube-nocookie.com", "www.youtube-nocookie.com":
		switch {
		case len(parts) == 1 && parts[0] == "watch":
			id = u.Query().Get("v")
		case len(parts) == 2 && (parts[0] == "shorts" || parts[0] == "embed" || parts[0] == "live"):
			id = parts[1]
		}
		if youtubeID.MatchString(id) {
			return YouTube, id, "", true
		}
	case "youtu.be":
		if len(parts) == 1 && youtubeID.MatchString(parts[0]) {
			return YouTube, parts[0], "", true
		}
	case "vimeo.com", "www.vimeo.com":
		if len(parts) >= 1 && len(parts) <= 2 && vimeoID.MatchString(parts[0]) {
			if len(parts) == 2 {
				if !vimeoHash.MatchString(parts[1]) {
					return "", "", "", false
				}
				hash = parts[1]
			}
			return Vimeo, parts[0], hash, true
		}
	case "player.vimeo.com":
		if len(parts) == 2 && parts[0] == "video" && vimeoID.MatchString(parts[1]) {
			hash = u.Query().Get("h")
			if hash != "" && !vimeoHash.MatchString(hash) {
				hash = ""
			}
			return Vimeo, parts[1], hash, true
		}
	}
	return "", "", "", false
}

// valid says whether the parts are what ParseVideoURL would return.
func valid(provider, id, hash string) bool {
	switch provider {
	case YouTube:
		return youtubeID.MatchString(id) && hash == ""
	case Vimeo:
		return vimeoID.MatchString(id) && (hash == "" || vimeoHash.MatchString(hash))
	}
	return false
}

// PageURL is the canonical link to the video's page.
func PageURL(provider, id, hash string) string {
	if !valid(provider, id, hash) {
		return ""
	}
	if provider == YouTube {
		return "https://www.youtube.com/watch?v=" + id
	}
	if hash != "" {
		return "https://vimeo.com/" + id + "/" + hash
	}
	return "https://vimeo.com/" + id
}

// PlayerURL is the address of the embedded player. It uses the privacy hosts of both
// services, and is empty for parts that do not pass the checks.
func PlayerURL(provider, id, hash string) string {
	if !valid(provider, id, hash) {
		return ""
	}
	if provider == YouTube {
		return "https://www.youtube-nocookie.com/embed/" + id + "?rel=0&playsinline=1"
	}
	s := "https://player.vimeo.com/video/" + id + "?dnt=1"
	if hash != "" {
		s += "&h=" + hash
	}
	return s
}

// EmbedID is the item id of a video: yt-<id> or vimeo-<digits>.
func EmbedID(provider, id string) string {
	if provider == YouTube {
		return "yt-" + id
	}
	return "vimeo-" + id
}

// PlayerFor builds the player address from a link written in a file, or "" when the link is
// not a YouTube or Vimeo video.
func PlayerFor(raw string) string {
	p, id, h, ok := ParseVideoURL(raw)
	if !ok {
		return ""
	}
	return PlayerURL(p, id, h)
}
