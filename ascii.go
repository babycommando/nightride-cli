package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var asciiArt = `                        
 ::::...: :@@@ . @@%+.   .:::: 
 ::.  .   .@%% . #@@@@@%.  .:: 
 :.  #@@:  @%% .     -@@@%  .: 
 . :@@@@@@.%#% .:::.   :@@@: . 
  .@@@..@@@##% .::::::  .@@@.  
  %@@.   :@@%% ......... .@%%  
 .@%#  :.  :@@ ..-:.......=#@. 
 :@%- .:::   : . -@@@@@@@@@@@= 
 .@%#  :::::       :@@#: ...:: 
  %@@. .:::  :@*@-   :@@@+  .. 
  .@@@:.:   %@   +%    *@@@- : 
 . :@@..  :@+  -..@@:   .%@: : 
 :.     :%@%@@ =:.@%@%.     .: 
 ::.. ::@%***@:  :%**%@:: ..:: 
 .. :.%@#****#@= +%***#@%.: .. 
`

var rektArt = `                       
  ████████████████████████ 
     █████████████████████ 
 ███   ████████      █████ 
 █████   ███████     █████ 
 ███████   ████████  █████ 
 █████████    ████████████ 
 ███████████    ██████████ 
 █████████████    ████████ 
 █████  ████████    ████ 
 █████    ████████      
 █████      ████████    ██ 
 █████        ████████████ 
 █████          ██████████ 
 █████            ████████
`

var EasterEgg = `
▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓█████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓██████▓▒▒▒▒▒▒▒▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓██████▓▒▒▒▒▒░   ░▒▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓██████▓▓▒▒▒▓▓▓▓▒▒▒▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓█████▓▓▒▒▒▒░▒▒▒▒▒▓█▓▒▒▒▒▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓████▓▒▒▒▒▒▒      ▒█▒ ░▒▒▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓██▓▒▒▒▒▒▒██░     ▒█▒ ░▒▒▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒▒▒▒▒▒████████████▓▒▒▒▒▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒▒▒▒▒▒▒▒▒░         ░▓▒▒▒▒▒▓▓skill issue▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒▒▒▒▒▒▒▓▒▒▒░       ░▓▒▒▒▒▒▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒▒▒▒▒▒▒▒▓▓▓▒░░░░  ░▒▒▒▒▒▒▒▒▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒▒▒▒▒▒▒▒▒▓▓▓▓▒░░   ▒▓▓▒▒▒▒▒▒▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒▒▒▓▓▓▒▒▒▒▓██▓▒░   ▒█▓▒▒▒▒▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒▓████▓▒▒▒████▒░░░▓█▓▒▒▒▓██▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓▓▓▓▓█████████▒▒██████████▓▒▓██████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓▓▓████████████████████████████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓▓▓██████████████████████████████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓▓▓██████████████████████████████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓▓▓██████████████████████████████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓▓████████████████████████████████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓███████████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓██████████████▓░             ░████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓███████████████████████████▒ ▒██████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓█████████████████████████▒ ▒████████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓███████████████████████░ ▒██████████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓█████████████████████▓▒▓▓███████████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓███████████████████▓▓▓▓█████████████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓█████████▓▓██████▓▓▒▒▓▓▓▓▓▓▓▓▓▓▓████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓█████████▓▓██████▓▒▒▒▒▒▒▒▒▒▒▒▒▒▓████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓█████████▓▓█████████████████████████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓█████████▓▓█████████████████████████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓████████████████████████████████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓████████████████████████████████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
▓▓▓▓▓▓▓▓▓▓████████████████████████████████████▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
`

var RektoryArt = `
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⠀⢀⡠⢄⠜⠉⠲⢤⡀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠠⡏⠓⠋⡀⣀⣠⣄⠀⠀⠙⢦⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢰⣓⣤⡊⠀⠀⠀⣀⠁⠈⢱⠄⢧⠀⠀⠀⠀⠀
⠀⠀⠀⠀⣀⡀⠀⠀⠀⠀⠀⠀⢠⡏⢠⠀⢠⠀⢠⡌⠀⢰⣧⡀⡾⠀⠀⠀⠀⠀
⠀⠀⢠⠏⠁⢨⠇⠀⠀⠀⠀⠀⢸⠀⠈⢰⡃⠀⠈⠁⠀⠸⣿⣿⠃⠀⠀⠀⠀⠀
⠀⠀⠸⡄⠀⣿⠀⠀⠀⠀⠀⠀⢸⡰⣆⠤⠤⠄⣲⡄⠀⠀⠀⣹⠀⠀⠀⠀⠀⠀
⣰⠒⠒⠓⠤⡈⠳⣄⠀⠀⠀⠀⠀⢳⡀⠉⠍⠉⠀⠀⢀⡴⠋⠁⠀⠀⠀⠀⠀⠀
⡹⠒⠒⠠⢄⡸⠀⢹⠓⠤⢄⣀⠤⠴⠟⠢⣄⣀⠀⣀⠝⠉⡟⠲⢤⡀⠀⠀⠀⠀
⢱⠤⠤⠤⢄⡰⠠⢉⠆⠀⠀⠀⠀⠀⠈⠢⣈⡉⠉⠀⣠⠜⠀⠀⠀⠉⠳⣄⠀⠀
⠸⢄⡒⠤⠤⣃⠜⡜⠀⠀⠀⠀⠀⡀⠀⠀⠀⢸⠀⠀⢃⠀⠀⡄⠀⠀⠀⠈⢣⡀
⠀⠀⠉⠉⠉⠓⠺⠤⠤⠤⠤⠖⡏⠀⠀⠀⠀⠀⡇⠀⠘⠀⠀⢡⠀⡄⠀⠀⠀⢱
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⡇⠀⠀⠀⠀⠀⠀⠀⠀⡇⠀⠨⡜⠀⠀⠀⢀⡎
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠱⢄⡀⠀⠀⠀⢸⠀⠀⢁⡠⠖⡇⣀⠀⢀⠞⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⡆⠀⠈⠉⠐⠒⠃⠀⠀⠀⠀⠀⡇⠀⢱⠋⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⡗⠦⣀⡀⠀⠀⠀⠀⣀⡠⠔⠋⢱⢐⡏⠀⠀⠀
`

var EbsmArt = `
=:.....#=.:.-#:....*-..:-=.
 -:        :--=    + =    =
 -:    *:-:+:.=          .=
 -:       .=  =    *=  -*-.
 -:    *:=.+=:=           =
 -:    * .--:==    + =    =
=:          :#:         :=
 .-+=::#::==-#::::=:.+::::+
.=          .=-:   =+*   -:
-:     .==-+=:=+   :=*   -:
.=:         --=+:   :*   -:
:=+:+=::     =+=#   #=   -:
-:          :+=-=   .=   -:
:-::::%=:::=:+:.:#=-#:...:+
`

// StationColors maps station icon keys to their color schemes
var StationColors = map[string][]string{
	"nrfm":       {"#ff386f", "#d52d2d"}, // Pink to Purple (current default)
	"darksynth":  {"#af0000", "#FF0000"}, // Dark Red to Red
	"chillsynth": {"#00CED1", "#ffcba6"}, // Dark Turquoise to Sky Blue
	"datawave":   {"#ffe696", "#ffa200"}, // Lime to Lime Green
	"ebsm":       {"#ffffff", "#666666"}, // Orange Red to Gold
	"horrorsynth":{"#a200ff", "#5503cf"}, // Purple to Indigo
	"rekt":       {"#ffffff", "#f31111"}, // Deep Pink to Hot Pink
	"rektory":    {"#ffffff", "#ff386f"}, // Crimson to Fire Brick
	"spacesynth": {"#08637A", "#AA1149"}, // Midnight Blue to Royal Blue
}

func asciiArtWidth() int {
	max := 0
	for _, line := range strings.Split(asciiArt, "\n") {
		if len(line) > max {
			max = len(line)
		}
	}
	return max
}

func asciiArtHeight() int {
	return len(strings.Split(asciiArt, "\n"))
}

// CreateGradientForStation creates a vertical gradient for the given station
func CreateGradientForStation(iconKey string) func(int) string {
	colors, exists := StationColors[iconKey]
	if !exists {
		// Default to the original pink-purple gradient
		colors = []string{"#ff386f", "#7d3cff"}
	}
	
	topColor := colors[0]
	bottomColor := colors[1]
	height := asciiArtHeight()
	
	return func(y int) string {
		// Parse hex colors
		r1, g1, b1 := parseHexColor(topColor)
		r2, g2, b2 := parseHexColor(bottomColor)
		
		// Linear interpolation
		t := float64(y) / float64(height-1)
		r := int(float64(r1)*(1-t) + float64(r2)*t)
		g := int(float64(g1)*(1-t) + float64(g2)*t)
		b := int(float64(b1)*(1-t) + float64(b2)*t)
		
		return fmt.Sprintf("#%02x%02x%02x", r, g, b)
	}
}

// parseHexColor converts hex color string to RGB values
func parseHexColor(hex string) (int, int, int) {
	var r, g, b int
	if len(hex) == 7 && hex[0] == '#' {
		fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
	}
	return r, g, b
}

// RenderVisualizedASCII renders the ASCII art with visualization bars and gradient
func RenderVisualizedASCII(barHeights []int, iconKey string) string {
	//conditionally render ascii based on station
	var art string
	if iconKey == "rekt" {
		art = rektArt
	} else if iconKey =="rektory" {
		art = RektoryArt
	} else if iconKey =="ebsm" {
		art = EbsmArt
	} else {
		art = asciiArt
	}
	artLines := strings.Split(art, "\n")

	height := len(artLines)
	gradient := CreateGradientForStation(iconKey)
	
	visualLines := make([]string, height)
	for y := 0; y < height; y++ {
		line := []rune(artLines[y])
		for x := 0; x < len(line) && x < len(barHeights); x++ {
			if line[x] != ' ' && barHeights[x] >= height-y {
				line[x] = '░'
			}
		}
		visualLines[y] = lipgloss.NewStyle().
			Foreground(lipgloss.Color(gradient(y))).
			Render(string(line))
	}
	
	return strings.Join(visualLines, "\n")
}
