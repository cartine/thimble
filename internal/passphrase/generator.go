// Package passphrase generates human-readable secrets without exposing them
// to a browser or command line argument.
package passphrase

import (
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"strings"
)

const (
	PresetHyphenated4WGT30C = "hyphenated-4w-gt30c"
	WordCount               = 4
	MinimumLength           = 31
)

// Generate returns four hyphen-separated compound words. Each compound is
// selected from two independent 128-entry lists, yielding 56 bits of entropy.
func Generate() (string, error) {
	return generateFrom(rand.Reader)
}

func generateFrom(reader io.Reader) (string, error) {
	words := make([]string, WordCount)
	for i := range words {
		left, err := randomEntry(reader, wordStarts)
		if err != nil {
			return "", fmt.Errorf("generate passphrase: %w", err)
		}
		right, err := randomEntry(reader, wordEnds)
		if err != nil {
			return "", fmt.Errorf("generate passphrase: %w", err)
		}
		words[i] = left + right
	}
	phrase := strings.Join(words, "-")
	if len(phrase) < MinimumLength {
		return "", fmt.Errorf("generated passphrase is shorter than %d characters", MinimumLength)
	}
	return phrase, nil
}

func randomEntry(reader io.Reader, entries []string) (string, error) {
	index, err := rand.Int(reader, big.NewInt(int64(len(entries))))
	if err != nil {
		return "", err
	}
	return entries[index.Int64()], nil
}

var wordStarts = []string{
	"amber", "anchor", "apple", "april", "arbor", "arctic", "arrow", "atlas",
	"autumn", "badger", "bamboo", "beacon", "berry", "birch", "bison", "blossom",
	"bluebird", "border", "bronze", "brook", "canyon", "cedar", "cherry", "cinder",
	"citrus", "clover", "cobalt", "comet", "coral", "cotton", "coyote", "crimson",
	"crystal", "dawn", "desert", "drift", "eagle", "earth", "ember", "falcon",
	"feather", "field", "firefly", "forest", "fossil", "frost", "garden", "glacier",
	"golden", "gravel", "harbor", "hazel", "heron", "hollow", "honey", "island",
	"ivory", "jasper", "juniper", "kestrel", "lagoon", "lantern", "laurel", "lemon",
	"lilac", "lotus", "lunar", "maple", "meadow", "meteor", "midnight", "misty",
	"mossy", "mountain", "nectar", "night", "north", "ocean", "olive", "onyx",
	"orchard", "otter", "pebble", "pepper", "pine", "planet", "plum", "prairie",
	"quartz", "raven", "river", "robin", "rocket", "rosewood", "rowan", "ruby",
	"saddle", "saffron", "scarlet", "shadow", "silver", "skyward", "solar", "sparrow",
	"spring", "spruce", "starling", "stone", "storm", "summer", "sunset", "swift",
	"timber", "topaz", "trail", "tulip", "tundra", "valley", "velvet", "violet",
	"walnut", "willow", "winter", "wolf", "wonder", "woodland", "yellow", "zephyr",
}

var wordEnds = []string{
	"bank", "bark", "beam", "bell", "berry", "bird", "bloom", "board",
	"branch", "breeze", "bridge", "brook", "candle", "castle", "cave", "circle",
	"cliff", "cloud", "coast", "crane", "creek", "crest", "crown", "dale",
	"dance", "dawn", "dewfall", "dream", "drift", "drop", "feather", "fern",
	"field", "finch", "fire", "flame", "flight", "flower", "forge", "forest",
	"foxglove", "garden", "gate", "glade", "glow", "grove", "harbor", "haven",
	"hawk", "heart", "hill", "hollow", "horse", "hound", "house", "isle",
	"jasper", "keeper", "kite", "lake", "lamb", "land", "leaf", "light",
	"lily", "lodge", "marsh", "meadow", "mist", "moon", "moss", "nest",
	"night", "north", "orchard", "path", "peak", "pine", "plume", "pond",
	"rain", "reef", "ridge", "rise", "river", "road", "rock", "root",
	"rose", "sail", "shade", "shell", "shore", "skyline", "song", "spark",
	"spring", "star", "stone", "storm", "stream", "sunrise", "tail", "thorn",
	"tide", "tower", "trail", "tree", "vale", "view", "walk", "water",
	"wave", "wayfarer", "whistle", "wind", "wing", "wood", "work", "wren",
	"yard", "zephyr", "harvest", "lantern", "lotus", "maple", "meteor", "willow",
}
