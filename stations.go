package main

import (
	"strings"
)

/* ─────────────  Station Data  ───────────── */

type station struct {
	name, url string
	title     string
	listeners int
}

var stations = []station{
	{name: "Nightride FM", url: "https://stream.nightride.fm/nightride.mp3"},
	{name: "Darksynth", url: "https://stream.nightride.fm/darksynth.mp3"},
	{name: "Chillsynth", url: "https://stream.nightride.fm/chillsynth.mp3"},
	{name: "Datawave", url: "https://stream.nightride.fm/datawave.mp3"},
	{name: "EBSM", url: "https://stream.nightride.fm/ebsm.mp3"},
	{name: "Horrorsynth", url: "https://stream.nightride.fm/horrorsynth.mp3"},
	{name: "Spacesynth", url: "https://stream.nightride.fm/spacesynth.mp3"},
	{name: "Rekt", url: "https://stream.nightride.fm/rekt.mp3"},
	{name: "Rektory", url: "https://stream.nightride.fm/rektory.mp3"},
}

func (s station) Title() string       { return s.name }
func (s station) Description() string { return s.title }
func (s station) FilterValue() string { return s.name }
func (s station) id() string {
	key := strings.ToLower(stationKey(s.url))
	return strings.TrimSuffix(key, ".mp3")
}

func stationKey(u string) string {
	return u[strings.LastIndex(u, "/")+1:]
}
