# nightride-cli

Nightride FM - the synthwaviest radio now inside your terminal, hackerman!!

![Demo](assets/demo.gif)

This client establishes a direct uplink to Nightride FM via global packet infrastructure (internet). Audio is streamed, decoded locally and sent directly to your machine's audio output using low-level access through Go's sound libraries.

Built for the command line frontier.

## Fast Installation Methods

## Build Instructions (It's very fast)

1. Make sure you have [Go](https://go.dev/doc/install) installed.

If you are on Linux, make sure to install ALSA (otherwise skip this step)

```bash
# Ubuntu/Debian
sudo apt install libasound2-dev
# Arch
sudo pacman -S alsa-lib
# Fedora
sudo dnf install alsa-lib-devel
```

2. Install dependencies:

```bash
go mod tidy
```

3. Run locally:

```bash
go run main.py
```

4. Build binary:

```bash
# Windows
go build -o nightride.exe
# Linux/macOS
go build -o nightride
```

5. Add to PATH (so you can run nightride from anywhere):
   🪟 **Windows**
   Move nightride.exe to a folder like C:\nightride, then:
   Press ⊞ Win → search "Environment Variables"
   Edit PATH, add: C:\nightride

🐧 **Linux**

```bash
mv nightride ~/.local/bin
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

🍎 **MacOS**

```bash
mv nightride ~/.local/bin
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

Done. Now type nightride in any terminal. ✅

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
