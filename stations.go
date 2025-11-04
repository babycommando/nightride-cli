package main

import (
	"strings"
)

/* ─────────────  Station Data  ───────────── */

type station struct {
	name, url string
	title     string
	listeners int
	youtube   string // empty if no video source
}

var stations = []station{
	{name: "Nightride FM", url: "https://stream.nightride.fm/nightride.mp3", youtube: "https://www.youtube.com/watch?v=uYfxDF_QR94"},
	{name: "Darksynth",   url: "https://stream.nightride.fm/darksynth.mp3",   youtube: "https://youtu.be/Nn87x5B26-c"},
	{name: "Chillsynth",  url: "https://stream.nightride.fm/chillsynth.mp3",  youtube: "https://youtu.be/UedTcufyrHc"},
	{name: "Datawave",    url: "https://stream.nightride.fm/datawave.mp3",    youtube: "https://youtu.be/Y9q6RYg2Pdg"},
	{name: "EBSM",        url: "https://stream.nightride.fm/ebsm.mp3",        youtube: "https://youtu.be/1PkJmurhQfU"},
	{name: "Horrorsynth", url: "https://stream.nightride.fm/horrorsynth.mp3", youtube: ""}, // none provided
	{name: "Spacesynth",  url: "https://stream.nightride.fm/spacesynth.mp3",  youtube: "https://youtu.be/5-anTj1QrWs"},
	{name: "Rekt",        url: "https://stream.nightride.fm/rekt.mp3",        youtube: ""}, // none provided
	{name: "Rektory",     url: "https://stream.nightride.fm/rektory.mp3",     youtube: ""}, // none provided
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