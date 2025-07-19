# nightride-cli

Nightride FM - the synthwaviest radio now inside your terminal, hackerman!!

![Demo](assets/demo.gif)

This client establishes a direct uplink to Nightride FM via global packet infrastructure (internet). Audio is streamed, decoded locally and sent directly to your machine's audio output using low-level access through Go's sound libraries.

Built for the command line frontier.

## Installation

1. ### Download for your system

   If you can't see your system/distro, consider building the go project. More in the Build Instructions section.

   - Windows
   - Linux (Debian/Ubuntu)

2. ### If you are on Linux, make sure you have ALSA (otherwise skip this step)

```bash
# Ubuntu/Debian
sudo apt install libasound2-dev
```

3. ### Add to PATH (so you can run "nightride" command from anywhere):

- **Windows**

```
 Move nightride.exe to a folder like C:\nightride, then:
 Press ⊞ Win → search "Environment Variables"
 Edit PATH, add: C:\nightride
```

- **Linux**

```bash
mv nightride ~/.local/bin
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

- **MacOS**

```bash
mv nightride ~/.local/bin
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

4. Open a fresh terminal and type "nightride" 😎

## Build Instructions (It's very fast)

1. ### Make sure you have [Go](https://go.dev/doc/install) installed.

2. ### If you are on Linux, make sure you have ALSA (otherwise skip this step)

```bash
# Ubuntu/Debian
sudo apt install libasound2-dev
# Arch
sudo pacman -S alsa-lib
# Fedora
sudo dnf install alsa-lib-devel
```

3. ### Install dependencies:

```bash
go mod tidy
```

4. ### Run locally:

```bash
go run main.py
```

5. ### Build binary:

```bash
# Windows
go build -o nightride.exe
# Linux/macOS
go build -o nightride
```

6. ### Add to PATH so you can run nightride from anywhere
   (instructions are the same for the quick installation)

Done. Now type nightride in any terminal.

---

```
 ..     %@@ .@@@@=   ..
 .  +@= -@@    -@@@#
  -@@@@@=@@ ..    #@@-
  @@= =@@@@ .....  =@@
 =@@    =@@  .      +@=
 #@* ..   =  +@@@@@@@@@
 =@@  ..   *#  =@@=
  @@+ .  -@.*@-  =@@@
  -@@   +@.   @*   #@=
      =@@@@.  @@@=
 .  -#@@##@@ =@#%@@-  .
  --===---=--==--===--
```
