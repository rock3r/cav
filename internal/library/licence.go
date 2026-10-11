package library

import (
	"regexp"
	"strings"
)

// Terms are what a licence allows, for the checks.
type Terms struct {
	ID                  string // SPDX-like id: CC0-1.0, CC-BY-4.0, CC-BY-NC-SA-3.0, PDM, Pexels, Unsplash
	URL                 string
	AttributionRequired bool
	CommercialOK        bool
	ShareAlike          bool
	NoDerivatives       bool
	// Note explains the terms when "non-commercial" alone would mislead (YuE2).
	Note string
}

var ccURL = regexp.MustCompile(`creativecommons\.org/(licenses|publicdomain)/([a-z-]+)/([0-9.]+)`)

// FromCC reads a Creative Commons code ("by-nc", "cc0", "pdm") and version.
func FromCC(code, version string) Terms {
	code = strings.ToLower(strings.TrimSpace(code))
	switch code {
	case "cc0", "zero":
		return Terms{ID: "CC0-1.0", URL: "https://creativecommons.org/publicdomain/zero/1.0/", CommercialOK: true}
	case "pdm", "mark":
		return Terms{ID: "PDM", URL: "https://creativecommons.org/publicdomain/mark/1.0/", CommercialOK: true}
	}
	if version == "" {
		version = "4.0"
	}
	t := Terms{
		ID:                  "CC-" + strings.ToUpper(code) + "-" + version,
		URL:                 "https://creativecommons.org/licenses/" + code + "/" + version + "/",
		AttributionRequired: strings.HasPrefix(code, "by"),
		CommercialOK:        !strings.Contains(code, "nc"),
		ShareAlike:          strings.Contains(code, "sa"),
		NoDerivatives:       strings.Contains(code, "nd"),
	}
	if !strings.HasPrefix(code, "by") && !strings.HasPrefix(code, "nc") {
		return Terms{ID: "unknown"}
	}
	return t
}

// FromURL reads a Creative Commons licence URL.
func FromURL(u string) Terms {
	m := ccURL.FindStringSubmatch(strings.ToLower(u))
	if m == nil {
		return Terms{ID: "unknown", URL: u}
	}
	return FromCC(m[2], m[3])
}

// FromName reads names such as "CC BY-SA 4.0", "CC0", "Public domain", "Attribution
// NonCommercial", "Creative Commons 0".
func FromName(name string) Terms {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case n == "":
		return Terms{ID: "unknown"}
	case strings.Contains(n, "cc0"), n == "creative commons 0", strings.Contains(n, "cc-zero"):
		return FromCC("cc0", "")
	case strings.Contains(n, "public domain"), n == "pd", strings.HasPrefix(n, "pd-"):
		return FromCC("pdm", "")
	}
	version := ""
	if m := regexp.MustCompile(`([0-9]\.[0-9])`).FindString(n); m != "" {
		version = m
	}
	var parts []string
	has := func(s ...string) bool {
		for _, x := range s {
			if strings.Contains(n, x) {
				return true
			}
		}
		return false
	}
	if has("by", "attribution") {
		parts = append(parts, "by")
	}
	if has("nc", "noncommercial", "non-commercial") {
		parts = append(parts, "nc")
	}
	if has("nd", "noderiv", "no deriv") {
		parts = append(parts, "nd")
	}
	if has("sa", "sharealike", "share alike", "share-alike") {
		parts = append(parts, "sa")
	}
	if len(parts) == 0 || parts[0] != "by" {
		return Terms{ID: "unknown"}
	}
	return FromCC(strings.Join(parts, "-"), version)
}

// Fixed licences of stock sites.
var (
	PexelsTerms   = Terms{ID: "Pexels", URL: "https://www.pexels.com/license/", CommercialOK: true}
	UnsplashTerms = Terms{ID: "Unsplash", URL: "https://unsplash.com/license", CommercialOK: true}
)

// YuE2Terms are the terms of a take made with YuE2's weights (MODEL_LICENSE in the YuE
// repository, read on 2026-10-11). The weights are CC BY-NC 4.0. A separate permission lets
// personal users, creators and musicians acting as individuals monetize what they make,
// and says outputs are not non-commercial only because YuE2 made them. It does not cover a
// company: a company that uses the weights commercially needs a licence from the authors.
// cav cannot tell an individual from a company, so a take counts as non-commercial: an
// "nc-ok" project accepts it, and a "commercial" project gets a warning that says who may
// still use it. Credit is optional but encouraged.
var YuE2Terms = Terms{ID: "YuE2-model-licence", URL: "https://github.com/multimodal-art-projection/YuE/blob/main/MODEL_LICENSE",
	Note: "made with YuE2, whose weights are CC BY-NC 4.0: an individual creator or musician may use and sell the music, " +
		"but a company needs a commercial licence from the YuE2 authors (ryuanab@connect.ust.hk)"}

// GeneratedTerms are the terms cav records for a take or image from a generation service:
// YuE2's model licence for yue2, and "generated" (commercial use allowed) for the others.
func GeneratedTerms(provider string) Terms {
	if provider == "yue2" {
		return YuE2Terms
	}
	return Terms{ID: "generated", CommercialOK: true}
}

// LicenceName turns an id into words for a credit line.
func LicenceName(id string) string {
	switch {
	case id == "CC0-1.0":
		return "CC0 1.0"
	case id == "PDM":
		return "Public Domain Mark"
	case strings.HasPrefix(id, "CC-"):
		rest := strings.TrimPrefix(id, "CC-")
		i := strings.LastIndex(rest, "-")
		if i > 0 {
			return "CC " + rest[:i] + " " + rest[i+1:]
		}
		return "CC " + rest
	case id == "Pexels":
		return "Pexels License"
	case id == "Unsplash":
		return "Unsplash License"
	case id == YuE2Terms.ID:
		return "YuE2 model licence"
	}
	return id
}

// Apply copies the terms into an entry.
func (t Terms) Apply(e *Entry) {
	e.Licence, e.LicenceURL = t.ID, t.URL
	e.AttributionRequired, e.CommercialOK, e.ShareAlike, e.NoDerivatives = t.AttributionRequired, t.CommercialOK, t.ShareAlike, t.NoDerivatives
	e.LicenceNote = t.Note
}

// Allowed reports whether the licence fits a filter: "any", "commercial" (no NC), or
// "nc-ok" (anything but unknown).
func (t Terms) Allowed(filter string) bool {
	switch filter {
	case "commercial":
		return t.CommercialOK && t.ID != "unknown"
	case "nc-ok":
		return t.ID != "unknown"
	}
	return true
}
