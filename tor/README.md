<div align="center">
  <img src="https://readme-typing-svg.demolab.com?font=Fira+Code&weight=600&size=28&pause=800&color=2196F3&center=true&vCenter=true&width=600&height=60&lines=IP+Changer" alt="Typing Animation" />
</div>

<p align="center">
  A bash script to cycle your public IP address through the Tor network automatically.
</p>

## About

A Linux utility that rotates your IP address by reloading Tor exit nodes. Set an interval and count, or let it run infinitely with randomized timing.

<div align="center">

![Bash](https://img.shields.io/badge/Bash-4EAA25?style=for-the-badge&logo=gnubash&logoColor=white)
![Linux](https://img.shields.io/badge/Linux-FCC624?style=for-the-badge&logo=linux&logoColor=black)
![Tor](https://img.shields.io/badge/Tor-7D4698?style=for-the-badge&logo=tor&logoColor=white)

</div>

## Features

<div align="center">

- **Automatic IP Rotation** — Reloads Tor service to get a new exit node IP
- **Custom Intervals** — Set your own time between IP changes (seconds)
- **Custom Count** — Specify how many times to change, or run infinitely
- **Infinite Mode** — Random 10–20s intervals for continuous cycling
- **Auto-install** — Detects your distro and installs curl + tor if missing
- **Distro Support** — Works on Debian/Ubuntu, Fedora/CentOS, and Arch
- **Colored Output** — Clean terminal UI with colored IP display

</div>

## Tech Stack

<div align="center">

| Component | Technology |
|-----------|------------|
| Language | Bash |
| Proxy | Tor (SOCKS5, port 9050) |
| IP Check | checkip.amazonaws.com |
| Init System | systemd |

</div>

## Getting Started

### Prerequisites

- A Linux distribution with `systemd`
- Root access (sudo)

### Usage

```bash
sudo bash ip-changer.sh
```

Or make it executable first:

```bash
chmod +x ip-changer.sh
sudo ./ip-changer.sh
```

The script will:
1. Install `curl` and `tor` if not present
2. Configure Tor with exit nodes in US, UK, France, and Germany
3. Show your current Tor IP
4. Ask for the time interval (seconds) between changes
5. Ask how many IPs to cycle through (0 = infinite)

## How It Works

1. Routes your traffic through the Tor SOCKS5 proxy on `127.0.0.1:9050`
2. Reloads the Tor service to force a new exit node
3. Checks the new public IP via `checkip.amazonaws.com`
4. Repeats based on your configured interval and count
