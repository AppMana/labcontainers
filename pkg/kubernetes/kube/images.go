package kube

import (
	"regexp"
	"sort"
	"strings"
)

// Both forms occur in a real manifest: image as a later key of a list item,
// and image as its first, where YAML puts the dash on the same line.
var imageLine = regexp.MustCompile(`(?m)^\s*-?\s*image:\s*(\S+)\s*$`)

// ImagesIn lists every image a manifest names, deduplicated and sorted, so a
// lab without a route to a registry can carry each one in before applying it.
// Digest pins are kept exactly; templated values a manifest does not resolve
// itself are skipped.
func ImagesIn(manifest []byte) []string {
	seen := map[string]bool{}
	for _, m := range imageLine.FindAllStringSubmatch(string(manifest), -1) {
		image := strings.Trim(m[1], `"'`)
		if image == "" || strings.Contains(image, "{{") {
			continue
		}
		seen[image] = true
	}
	out := make([]string, 0, len(seen))
	for image := range seen {
		out = append(out, image)
	}
	sort.Strings(out)
	return out
}
