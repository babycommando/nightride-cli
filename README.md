# nightride-cli

Nightride FM - the synthwaviest radio now inside your terminal, hackerman!!

![Demo](assets/demo.gif)

This client establishes a direct uplink to Nightride FM via global packet infrastructure (internet). Audio is streamed, decoded locally and sent directly to your machine's audio output using low-level access through Go's sound libraries.

Built for the command line frontier.

## Fast Installation Methods

## Build Instructions (It's very fast)

1. Make sure you have [Go](https://go.dev/doc/install) installed.

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
go build -o nightride
```

5. Finally, add the binary to your system path.

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
