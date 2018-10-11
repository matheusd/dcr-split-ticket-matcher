# Building

Build using go 1.11 and go modules.

Before building the GUI client, you'll probably need some gtk headers installed (see below for Windows instructions):

```
# fedora
$ sudo dnf install gtk2-devel

# debian/ubuntu
$ sudo apt-get install libgtk2.0-dev libglib2.0-dev libgtksourceview2.0-dev
```

For everything else, follow the standard instructions as other decred projects. This project is using go modules, so you should preferably build *outside* of your $GOPATH.

```
$ git clone https://github.com/matheusd/dcr-split-ticket-matcher
$ cd dcr-split-ticket-matcher
$ go build ./cmd/...
```

## Windows Build

For a Windows build you should use MSYS2. Follow the instructions at the [GTK Website](https://www.gtk.org/download/windows.php) (download and install MSYS2, then install GTK via `pacman`).

On an MSYS2 shell, export the following environment variable:

```
$ export CGO_LDFLAGS_ALLOW="-Wl,-luuid"
```

Then proceed using the usual build instructions for a go 1.11+ project.

# Running the Client

See instructions about [command line client](client-cli.md) or the [GUI client](client-gui.md).

# Running the Service

Copy the configuration file to the respective dir and configure it:

```
$ mkdir -p ~/.dcrstmd
$ cp samples/dcrstmd.conf ~/.dcrstmd
```

Run the executable. During development, it is easier to do the following:

```
$ go run cmd/dcrstmd/main.go
```

Run with `--publishtransactions` to automatically publish the split and ticket transactions once a session is successfully finished.
